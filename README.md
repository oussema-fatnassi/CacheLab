# CacheLab

CacheLab est un projet de cache clé/valeur en mémoire, développé en Go et destiné aux applications e-commerce.

## Prérequis

- [Go 1.27.3 ou une version compatible plus récente](https://go.dev/dl/).
- GNU Make et un compilateur C pour les tests avec détecteur de courses. Sur macOS, installer les outils en ligne de commande avec `xcode-select --install`.
- Docker avec le plugin Compose pour le lancement en conteneur.

## Démarrage rapide

Exécuter les commandes depuis la racine du projet, où se trouvent `go.mod` et le `Makefile`.

Au premier lancement, copier `.env.example` vers `.env`, puis modifier `PORT` si nécessaire. Si `.env` existe déjà, conserver ce fichier. Le port par défaut est 8080.

Au démarrage, le serveur construit une table et son service. Les variables `CACHE_BUCKET_COUNT`, `MAX_KEY_BYTES` et `MAX_VALUE_BYTES` définissent respectivement le nombre initial de buckets, la taille maximale d'une clé et celle d'une valeur en octets. Leurs valeurs par défaut provisoires sont 16, 256 et 1 048 576 ; elles restent à valider pour le projet ( a arbitrer en équipe). Elles doivent être des entiers strictement positifs.

Le port doit être compris entre 1 et 65535. `MAX_BODY_BYTES` limite le corps complet des requêtes POST et PUT (8 388 608 octets par défaut). Le corps inclut la structure JSON et les caractères échappés : adapter cette limite si nécessaire. `SHUTDOWN_TIMEOUT` fixe le délai d’arrêt progressif (`10s` par défaut, durée Go positive). Ces valeurs sont provisoires et à valider en équipe. Les quatre routes CRUD sont raccordées : `POST /keys`, `GET /keys/{key}`, `PUT /keys/{key}` et `DELETE /keys/{key}`.

```bash
cp .env.example .env
```



### Lancer le serveur

Dans un premier terminal :

```bash
make server
```

Le serveur écoute sur le port défini dans `.env`, ou sur `http://localhost:8080` par défaut. La route `/` répond avec `Hello, World!`. Garder ce terminal ouvert ; `Ctrl+C` arrête le serveur.

### Lancer le client

Dans un second terminal, depuis la même racine du projet :

```bash
make client
```

Le client envoie une requête GET sur `/`, affiche le statut HTTP et le contenu de la réponse, puis termine :

```text
200 OK
Hello, World!
```

Les commandes `make server` et `make client` chargent toutes deux `.env` : le client utilise donc le même port que le serveur. Par exemple, avec `PORT=1111`, il contacte `http://localhost:1111/`.

Un lancement direct avec `go run` ne charge pas automatiquement `.env`. Depuis `cmd/client`, fournir le port explicitement si nécessaire :

```bash
PORT=1111 go run .
```

Si le client affiche `connection refused`, vérifier que le serveur est démarré et que les deux programmes utilisent le même port.

### Lancer le serveur avec Docker

Pour le lancer avec Docker :

```bash
docker compose up --build
```

Compose utilise le port défini dans `.env`, ou 8080 par défaut. Le service est accessible sur ce même port depuis la machine hôte. Swagger UI est accessible sur le même port que l’API (voir [Documentation de l'API](#documentation-de-lapi-swagger)).

Autres commandes : `make build` construit `bin/cachelab`, `make test` exécute les tests avec le détecteur de courses, `make lint` lance `go vet`, `gofmt -w .` formate les fichiers Go et `make cover` affiche la couverture.

## Logs des requêtes

Le serveur écrit ses logs au format JSON sur la sortie standard. Chaque requête produit un événement `http_request` contenant `method`, `route`, `status`, `duration_ms` et `request_id`, ainsi que les champs standards `time`, `level` et `msg` de `slog`.

`route` est le motif de routage, par exemple `GET /keys/{key}`, et non l’URL réelle. Les corps, valeurs, clés complètes, paramètres d’URL et credentials ne sont pas journalisés. Les erreurs internes et panics produisent un message générique, sans leur contenu brut.

Un identifiant est généré par le serveur pour chaque requête et renvoyé dans l’en-tête `X-Request-ID`. Un identifiant fourni par le client est ignoré. `duration_ms` mesure le traitement côté serveur, pas le temps réseau complet.

Pour observer les logs, lancer `make server` puis envoyer des requêtes avec Swagger ou le client. Avec Docker, utiliser `docker compose logs -f cachelab`. Pour vérifier le middleware et le filtrage des données :

```bash
go test -race -v -count=1 ./cmd/cachelab -run '^(TestRequestLogging|TestResponseStatus)'
```

## Documentation de l'API (Swagger)

La spécification OpenAPI des routes se trouve dans `api/openapi.yaml` et est intégrée au serveur lors de sa compilation. Après avoir lancé le serveur localement ou avec Docker Compose, ouvrir `http://localhost:8080/swagger/` (adapter le port à `PORT`).

Swagger UI permet de consulter et de tester les routes directement dans le navigateur : déplier une route, cliquer sur **Try it out**, compléter les champs puis cliquer sur **Execute**. Les requêtes utilisent le même serveur et le même port que la documentation.

L'interface Swagger UI charge ses fichiers JavaScript et CSS depuis un CDN : une connexion Internet est nécessaire pour l'afficher. Après modification de `api/openapi.yaml`, reconstruire et redémarrer le serveur pour intégrer la nouvelle spécification.

### Format des erreurs

Toutes les réponses d'erreur ont le même corps JSON, avec l'en-tête `Content-Type: application/json` :

```json
{"error": "key not found"}
```

| Code | Cas | Message |
| --- | --- | --- |
| 400 | JSON invalide, clé vide ou trop longue, valeur trop volumineuse | `invalid JSON body`, `invalid request body`, `invalid key`, `value too large` |
| 404 | Clé absente | `key not found` |
| 409 | Clé déjà présente lors d'une création | `key already exists` |
| 413 | Corps plus grand que `MAX_BODY_BYTES` | `request body too large` |
| 500 | Erreur inattendue, y compris une panic dans un handler | `internal server error` |

Une erreur `500` ne contient jamais de détail interne : la cause réelle est écrite uniquement dans les logs du serveur, qui continue de traiter les requêtes suivantes.

## Tester le hachage

Modifier la chaîne `key` dans `TestPrintHash`, dans `internal/cache/hash_test.go`, puis lancer depuis la racine :

```bash
go test -v -count=1 ./internal/cache -run '^TestPrintHash$'
```

Le test affiche le hash en décimal et en hexadécimal, ainsi que l’index du bucket. Il permet d’observer le résultat.

## Gestion des collisions

La table calcule l’index d’un bucket avec `hash(key) % nombreDeBuckets`. Plusieurs clés différentes peuvent obtenir le même index : c’est une collision.

Les collisions sont gérées par **chaînage séparé** : chaque bucket pointe vers une liste d’entrées `CacheEntry`, reliées par le champ `next`. Une nouvelle entrée est ajoutée en tête de liste après vérification de l’absence de doublon.

```text
bucket → country: France → city: Paris → name: Test → nil
```

La lecture et la mise à jour parcourent la liste en comparant les clés. La suppression ajuste les pointeurs pour retirer uniquement l’entrée correspondante, sans perdre les autres. Une clé déjà présente est refusée avec `ErrKeyExists` ; une collision entre deux clés différentes est acceptée.

Pour vérifier ce comportement, les tests utilisent un seul bucket et un seuil de charge élevé pour éviter un redimensionnement pendant ce scénario. Ils y stockent trois clés et contrôlent la lecture de chaque valeur et la mise à jour ou suppression en début, milieu et fin de liste :

```bash
go test -v -count=1 ./internal/cache -run '^TestHashTableCollision$'
```

## Accès concurrents

Un `sync.Mutex` protège chaque opération CRUD de la HashTable. Une seule opération peut accéder à la table à la fois, y compris les lectures. La vérification d’un doublon et son insertion sont réalisées sous le même verrou.

Le redimensionnement utilise le verrou déjà pris par `Create` : les autres opérations attendent la fin de la redistribution des entrées. Une HashTable ne doit pas être copiée après sa première utilisation ; utiliser le pointeur retourné par son constructeur.

Les tests vérifient deux créations simultanées de la même clé, des lectures pendant les mises à jour et un mélange de CRUD avec collisions et redimensionnements. La CI exécute également les tests avec le détecteur de courses :

```bash
go test -race -v -count=1 ./internal/cache -run '^TestHashTableConcurrent'
```

## Redimensionnement automatique

Après une création, si le facteur de charge `size / nombreDeBuckets` dépasse **0,75**, la table double son nombre de buckets. Avec 4 buckets, 3 entrées ne déclenchent aucun changement ; la quatrième fait passer la capacité à 8 buckets.

Chaque entrée existante est déplacée vers son nouvel index, calculé avec le nouveau nombre de buckets. Les pointeurs `next` sont reconstruits pour conserver les collisions, sans modifier les valeurs ni le nombre d’entrées. Les suppressions ne réduisent pas la capacité.

`NewHashTable(bucketCount)` utilise ces valeurs par défaut. `NewHashTableWithConfig(bucketCount, maxLoadFactor, growthFactor)` permet de choisir un seuil positif et fini et un multiplicateur entier d’au moins 2.

Les tests vérifient le seuil, plusieurs agrandissements avec collisions, la conservation des données et le fonctionnement du CRUD après redimensionnement :

```bash
go test -v -count=1 ./internal/cache -run '^TestHashTableResize'
```
