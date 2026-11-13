# QA-05 — Contrôles CI et rapport de couverture

Ticket : [CL-123](https://oussemafatnassi.atlassian.net/browse/CL-123).

## Fonctionnement

Le job obligatoire `build-vet-lint` conserve son nom afin de rester associé à la protection de branche. Il exécute :

1. Vérification du formatage avec `gofmt`.
2. Tous les tests avec `go test -race -count=1 ./...`.
3. `make cover`, avec un objectif indicatif de 80 %, non bloquant.
4. `go vet`, golangci-lint **v2.15.0**, puis compilation.

Un test en échec ou une course détectée fait échouer l’étape de tests. Une couverture inférieure à 80 % affiche un avertissement sans faire échouer `make cover` ni la CI. Les erreurs de tests ou de génération du rapport restent bloquantes. Le job Windows existant reste présent pour les tests du client et de son horloge.

Les benchmarks et les campagnes HTTP restent manuels : leur vitesse ne décide pas si une PR est fusionnable. La CI n’exécute pas `make loadtest` et ne reçoit pas de credentials réels. Les clés utilisées dans les tests sont fictives. Le workflow conserve uniquement la permission `contents: read`.

## Périmètre de couverture

`make cover` mesure `./cmd/cachelab`, `./internal/cache` et `./api`, puis lance `tools/coverreport`. Le résultat global est :

`instructions couvertes / instructions totales × 100`

Ce n’est pas la moyenne des pourcentages des packages. Les fichiers de test ne comptent pas comme production. Les clients `cmd/client`, `cmd/loadtest` et l’outil `tools/coverreport` restent hors du calcul de couverture du serveur ; leurs tests font partie de la suite complète.

L’avertissement utilise les comptes exacts, avant arrondi : 79,999 % déclenche un avertissement même si l’affichage indique 80,00 %. Le taux reste informatif ; un profil vide ou invalide fait cependant échouer la commande.

GitHub Actions conserve `coverage.out` dans l’artefact `production-coverage` pendant sept jours, dès lors qu’un profil a été produit, même si une étape échoue. Un profil partiel après des tests en échec ne valide jamais le job : le code de sortie des tests reste bloquant.

## Démonstration du contrôle

Les cas suivants ont été reproduits localement avec des profils synthétiques, sans modifier la couverture réelle du projet :

| Profil | Résultat | Code de sortie de coverreport |
| --- | --- | --- |
| 4 instructions couvertes sur 5 : 80 % | Sans avertissement | 0 |
| 7 instructions couvertes sur 10 : 70 % | Avertissement non bloquant | 0 |

Ils sont également couverts, avec le cas sous le seuil malgré l’arrondi, par :

```bash
go test -v ./tools/coverreport -run '^TestReportWeightedCoverageAndWarning$'
```

Ces cas valident localement les avertissements et le code de sortie nul, même sous 80 %. La prochaine exécution CI après push validera le workflow complet sur cette révision.

## Protection de branche vérifiée

Vérification en lecture seule via l’API GitHub le **13 novembre 2026** :

- Ruleset **Main Protection** actif sur la branche par défaut, actuellement `main`.
- Contrôle **build-vet-lint** provenant de GitHub Actions obligatoire.
- Branche à jour, une approbation et résolution des conversations exigées.
- Aucune exception dans la liste de bypass ; force pushes et suppressions bloqués.

Ce ruleset cible `main`, pas `dev`. Il n’a pas été modifié par ce ticket. La décision de fusion nécessite toujours le résultat réel de la CI, en plus de ces paramètres.

## Limite de la vérification locale

Cet environnement interdit l’ouverture de ports TCP. Six tests réseau existants ne peuvent donc pas être exécutés ici : redémarrage du processus, requêtes OPTIONS/HEAD sur le réseau, arrêt progressif et port occupé. Ils restent activés en CI ; aucune exclusion n’est ajoutée au workflow ou au Makefile.

Les autres tests de production donnent localement **326/372 instructions couvertes, soit 87,63 %**, avec ces six tests exclus uniquement pour la vérification locale. Ce chiffre n’est pas présenté comme une mesure de la suite complète : la couverture du run CI sans exclusions reste la référence.

La suite avec `-race`, en excluant uniquement ces six tests réseau, passe. Le formatage, `go vet`, golangci-lint v2.15.0 et la compilation ont également été vérifiés avec succès.
