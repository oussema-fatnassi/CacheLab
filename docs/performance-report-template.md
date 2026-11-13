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
