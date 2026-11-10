# Contribuer à CacheLab

## Branches et commits

- Créer une branche depuis `main` pour chaque ticket : `feat/CFG-02` ou `fix/CL-123-description`.
- Utiliser un message de commit court au format `<type>(<scope>): <description>`, par exemple `chore(.golangci.yml): configure Go linting`.
- Ouvrir une pull request vers `main` et renseigner son résumé, son ticket et les tests effectués.

## Contrôles Go

Installer le binaire officiel de golangci-lint v2.15.0. Sur macOS, utiliser le terminal ; sur Windows, utiliser [Git Bash](https://git-scm.com/download/win) :

```bash
curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.15.0
```

Ajouter le répertoire `bin` de `GOPATH` au `PATH` s'il n'y figure pas déjà. Dans Git Bash ou le terminal macOS, pour la session en cours :

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Pour les prochains terminaux, ajouter cette ligne au fichier de configuration de son shell. Sous Windows, on peut également ajouter `C:\Users\<utilisateur>\go\bin` au `PATH` utilisateur, en adaptant le chemin si `GOPATH` a été modifié.

Vérifier la version installée, puis exécuter depuis la racine :

```bash
golangci-lint version
golangci-lint fmt
golangci-lint run
go test ./...
```

`golangci-lint fmt` applique `gofmt` ; `golangci-lint run` exécute les linters configurés dans `.golangci.yml`.
Voir la [documentation officielle d'installation](https://golangci-lint.run/docs/welcome/install/local/).

## Revue et protection de main

Chaque modification de `main` passe par une pull request approuvée par au moins une autre personne. L'auteur corrige les remarques et vérifie les tests avant la fusion. Ne pas pousser directement sur `main`.