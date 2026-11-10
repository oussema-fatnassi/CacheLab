# CacheLab

CacheLab est un projet de cache clé/valeur en mémoire, développé en Go et destiné aux applications e-commerce.

## Prérequis

- [Go 1.27.3 ou une version compatible plus récente](https://go.dev/dl/).
- GNU Make et un compilateur C pour les tests avec détecteur de courses. Sur macOS, installer les outils en ligne de commande avec `xcode-select --install`.
- Docker avec le plugin Compose pour le lancement en conteneur.

## Démarrage rapide

Exécuter les commandes depuis la racine du projet, où se trouvent `go.mod` et le `Makefile`.

Au premier lancement, copier `.env.example` vers `.env`, puis modifier `PORT` si nécessaire. Si `.env` existe déjà, conserver ce fichier. Le port par défaut est 8080.

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
