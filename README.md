# CacheLab

CacheLab est un projet de cache clé/valeur en mémoire, développé en Go et destiné aux applications e-commerce.

## Prérequis

- [Go 1.27.3 ou une version compatible plus récente](https://go.dev/dl/).

## Démarrage rapide

Depuis la racine du dépôt :

```bash
go build ./...
go test ./...
go run ./cmd/cachelab
```

Le projet contient pour l'instant un point d'entrée minimal : la commande `go run` se termine sans affichage et ne démarre pas encore de serveur. Aucun test n'est encore présent.
