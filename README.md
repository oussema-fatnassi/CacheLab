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

```bash
cp .env.example .env
```



### Lancer le serveur

Dans un premier terminal :

```bash
make run
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

Les commandes `make run` et `make client` chargent toutes deux `.env` : le client utilise donc le même port que le serveur. Par exemple, avec `PORT=1111`, il contacte `http://localhost:1111/`.

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

Compose utilise le port défini dans `.env`, ou 8080 par défaut. Le service est accessible sur ce même port depuis la machine hôte.

Autres commandes : `make build` construit `bin/cachelab`, `make test` exécute les tests avec le détecteur de courses, `make lint` lance `go vet`, `gofmt -w .` formate les fichiers Go et `make cover` affiche la couverture.

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

Pour vérifier ce comportement, les tests utilisent un seul bucket et y stockent trois clés. Ils contrôlent la lecture de chaque valeur et la mise à jour ou suppression en début, milieu et fin de liste :

```bash
go test -v -count=1 ./internal/cache -run '^TestHashTableCollision$'
```
