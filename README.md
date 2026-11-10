# CacheLab

CacheLab est un projet de cache clé/valeur en mémoire, développé en Go et destiné aux applications e-commerce.

## Prérequis

- [Go 1.27.3 ou une version compatible plus récente](https://go.dev/dl/).
- GNU Make et un compilateur C pour les tests avec détecteur de courses. Sur macOS, installer les outils en ligne de commande avec `xcode-select --install`.
- Docker avec le plugin Compose pour le lancement en conteneur.

## Démarrage rapide

Copier `.env.example` vers `.env`, puis modifier `PORT` si nécessaire. Le port par défaut est 8080.

```bash
cp .env.example .env
make run
```

Le serveur écoute sur `http://localhost:8080` par défaut. Il répond actuellement avec une erreur HTTP 404 : les routes de l'API seront ajoutées séparément.

Pour le lancer avec Docker :

```bash
docker compose up --build
```

Compose utilise le port défini dans `.env`, ou 8080 par défaut. Le service est accessible sur ce même port depuis la machine hôte.

Autres commandes : `make build` construit `bin/cachelab`, `make test` exécute les tests avec le détecteur de courses, `make lint` lance `go vet` et `make cover` affiche la couverture.
