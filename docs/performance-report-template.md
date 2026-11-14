# Comparaison simple des performances — Résultats

Aucun résultat enregistré pour le moment. Remplacer les tirets ci-dessous par les valeurs mesurées.

- Date / commit Git :
- Version de Go / machine / système :
- Mode de lancement (natif ou Docker) :
- Paramètres de journalisation / authentification (sans credentials) :
- Volume de journaux par essai (nombre d’événements `http request`) :
- Rapport JSON du client (les quatre exports `/metrics` et leurs dates figurent dans son champ `metrics`) :
- Valeurs : 100 octets par défaut ; concurrence réellement utilisée :
- Graine / débit proposé / échauffement / durée / pourcentage de misses :
- Seul paramètre modifié : nombre initial de buckets, 16 contre 1 024.

## Mesures

Dupliquer les lignes pour chaque volume : 1 000, 10 000 et 100 000 clés. Effectuer trois essais par configuration, avec un cache vide au début de chaque essai.

| Buckets initiaux | Clés N | Essai | Insertion totale (ms) | Lecture totale (ms) | Insertions réussies | Lectures réussies | Échecs de vérification / autres erreurs |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 16 | 1 000 | 1 | — | — | — | — | — |
| 16 | 1 000 | 2 | — | — | — | — | — |
| 16 | 1 000 | 3 | — | — | — | — | — |
| 1 024 | 1 000 | 1 | — | — | — | — | — |
| 1 024 | 1 000 | 2 | — | — | — | — | — |
| 1 024 | 1 000 | 3 | — | — | — | — | — |

Le rapport JSON conserve les latences individuelles, les débits, les statuts et les P50/P95/P99 des groupes `POST`, `GET_hit` et `GET_miss`. Recopier ces éléments si le scénario avancé est activé. Le temps moyen correspond à la moyenne des latences individuelles ; avec plusieurs workers ou un débit imposé, il ne faut pas le confondre avec le temps total divisé par N. La vérification exhaustive et l’échauffement sont exclus des échantillons mesurés.

Pour un débit imposé, joindre les créneaux prévus, requêtes envoyées, créneaux non envoyés, départs retardés et attente finale. Conserver les répétitions séparément ; ne jamais moyenner les percentiles pour annoncer un percentile global.

## Conclusions

- Quelle configuration a le temps médian d’insertion le plus faible pour chaque N ?
- Quelle configuration a le temps médian de lecture le plus faible ?
- Les écarts sont-ils cohérents entre les essais ?
- Y a-t-il eu des erreurs, interruptions ou autres limites ? Ne pas utiliser un essai en échec pour annoncer une configuration plus rapide.

Pour obtenir la médiane, trier les trois durées et retenir celle du milieu.

Ces mesures concernent les opérations HTTP complètes sur cette machine. Elles ne constituent pas une preuve de complexité O(1).


## Extension PERF-01-B — Résultats des trois moteurs

Protocole **B.1 — 14 novembre 2026** : [paramètres de l’étape B](performance-protocol.md#5-extension-perf-01-b--trois-moteurs). La grille précédente reste dédiée à l’étape A ; ne pas mélanger les deux expériences.

Compléter le contexte existant avec : moteur réellement sélectionné, hash, verrou, capacité initiale, versions Go serveur/client séparées, fonctionnalités actives, réglages Go/Docker, nombres d’opérations par phase et échauffement en opérations. Joindre les preuves de correction, authentification et redémarrage. Mesurer un seul moteur à la fois sur le même poste.

Dupliquer les lignes pour chaque moteur, volume (jusqu’à 1 000 000 clés), scénario et trois répétitions. Séparer création, lecture présente, lecture absente, modification et suppression, y compris dans les profils mixtes.

| Moteur / N / scénario / essai | Opération | Prévues / réussies / erreurs | Direct : ns/op, B/op, allocs/op | HTTP : total, P50/P95/P99, débit | Fichier brut |
| --- | --- | --- | --- | --- | --- |
| À renseigner | — | — | — | — | — |

| Point de collecte / moteur / N / essai | HeapAlloc / Sys serveur | RSS serveur / client | Delta CPU serveur / client | Preuve |
| --- | --- | --- | --- | --- |
| À renseigner | — | — | — | — |

Indiquer unités, outils et durée de phase pour le CPU. Relever à vide, après chargement, après échauffement, avant/après les phases et après suppression ; ne pas additionner les différentes mesures de mémoire.

Compléter les conclusions avec :

- Les trois P99 du test GET présent à 8 workers et le verdict < 10 ms, selon les conditions du protocole.
- La comparaison séparée des hashes custom et l’explication de complexité avec n/L/charge/d/c, moyenne, pire cas, amorti et sources Go.
- Les niveaux non exécutés, erreurs, ressources non mesurées, limites du générateur et changements depuis la première campagne. Un cas absent reste « non vérifié » ; conserver aussi les essais en échec.

Aucune durée minimale ni endurance n’est attendue. Le client doit encore être étendu dans QA-02-B pour produire l’ensemble de ces mesures.
