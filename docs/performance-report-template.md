# Comparaison simple des performances — Résultats

Aucun résultat enregistré pour le moment. Remplacer les tirets ci-dessous par les valeurs mesurées.

- Date / commit Git :
- Version de Go / machine / système :
- Mode de lancement (natif ou Docker) :
- Paramètres de journalisation / authentification (sans credentials) :
- Volume de journaux par essai (nombre d’événements `http request`) :
- Exports `/metrics` enregistrés avant / après chaque phase (noms des fichiers) :
- Valeurs : 100 octets ; un seul client séquentiel.
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

Temps moyen par opération = temps total de la phase en ms / N. Le calculer uniquement pour les essais où toutes les opérations attendues réussissent. Les lectures de vérification sont exclues des lectures chronométrées.

## Conclusions

- Quelle configuration a le temps médian d’insertion le plus faible pour chaque N ?
- Quelle configuration a le temps médian de lecture le plus faible ?
- Les écarts sont-ils cohérents entre les essais ?
- Y a-t-il eu des erreurs, interruptions ou autres limites ? Ne pas utiliser un essai en échec pour annoncer une configuration plus rapide.

Pour obtenir la médiane, trier les trois durées et retenir celle du milieu.

Ces mesures concernent les opérations HTTP complètes sur cette machine. Elles ne constituent pas une preuve de complexité O(1).
