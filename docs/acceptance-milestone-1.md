# QA-04-S2 — Recette minimale du premier jalon

Ticket : [CL-154](https://oussemafatnassi.atlassian.net/browse/CL-154).

Objectif : vérifier la version actuelle avec les tests et le client existants, puis conserver les preuves. Aucun nouvel outil n’est nécessaire.

## 1. Identifier la version et vérifier la qualité

```bash
git rev-parse HEAD
go version
gofmt -l .
golangci-lint run
go vet ./...
go build ./...
make test
make cover
```

`gofmt -l .` ne doit rien afficher. La couverture reste informative. `make test` couvre notamment CRUD, erreurs, authentification, métriques, logs et redémarrage réel du serveur.

Noter la date, le commit, les éventuelles modifications locales, la machine et le système, le mode de lancement et les paramètres de `.env` sans copier `API_KEY`. Conserver les sorties des contrôles ou un lien vers la CI du même commit.

## 2. Exécuter les trois charges

Renseigner `API_KEY` dans `.env` et garder la même configuration pour les trois essais. Dans un premier terminal :

```bash
make server
```

Dans un second terminal, exécuter chaque commande séparément :

```bash
make loadtest ARGS="-count 1000 -seed 42 -server-commit <COMMIT> -label qa04s2"
make loadtest ARGS="-count 10000 -seed 42 -server-commit <COMMIT> -label qa04s2"
make loadtest ARGS="-count 100000 -seed 42 -server-commit <COMMIT> -label qa04s2"
```

Remplacer `<COMMIT>` par le résultat de `git rev-parse HEAD`. **Entre chaque commande, arrêter le serveur avec Ctrl+C puis relancer `make server`** pour repartir avec un cache vide. Faire de même avant le premier essai si le serveur contient déjà des données. Le client ne vide pas le cache.

Paramètres par défaut : valeurs de 100 octets, concurrence 1, débit libre, aucun échauffement, N lectures et 0 % de misses. Chaque essai doit terminer sans erreur avec N insertions, N vérifications et N lectures réussies. Les JSON de `results/` contiennent les temps et les exports de métriques ; joindre les trois fichiers et les logs serveur utiles au ticket, sans secrets. `results/` est ignoré par Git.

| Clés | Résultat PASS/FAIL | Rapport JSON / réserve |
| --- | --- | --- |
| 1 000 | À exécuter | — |
| 10 000 | À exécuter | — |
| 100 000 | À exécuter | — |

## 3. Revue de l’équipe

| Responsable | Vérification | Validation / preuve |
| --- | --- | --- |
| P1 | Métriques cohérentes et logs sans données sensibles | À renseigner |
| P2 | Trois charges réussies et rapports identifiés | À renseigner |
| P3 | API/authentification, lancement Docker selon le README et cache vide après redémarrage | À renseigner |

Chaque échec reste une réserve avec résultat attendu, résultat observé et preuve. La recette attend les trois rapports et les validations de l’équipe ; ne pas clôturer le parent QA-04.

Limite de la préparation : les charges TCP et Docker ne peuvent pas être exécutés dans l’environnement de l’assistant. La [CI réussie du commit b1ff35f](https://github.com/oussema-fatnassi/CacheLab/actions/runs/43491009108) fournit les contrôles automatisés de cette version, mais ne remplace pas les trois campagnes ni la revue collective.
