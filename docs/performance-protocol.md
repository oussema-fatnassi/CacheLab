# PERF-01-A — Comparaison simple des performances

Objectif : charger des données, mesurer les temps d’insertion et de lecture, puis comparer deux configurations de notre table de hachage actuelle.

Ce document remplace le précédent brouillon détaillé. Aucun résultat n’a encore été mesuré. Le client automatisé sera développé dans QA-02-A.

## 1. Paramètres de l’expérience

| Paramètre | Valeur |
| --- | --- |
| Tailles des jeux de données | 1 000, 10 000 et 100 000 clés |
| Clés | `bench-0`, `bench-1`, etc. |
| Valeurs | La même chaîne de 100 caractères ASCII `x` pour chaque clé |
| Clients | Un client, envoyant une seule requête à la fois |
| Configuration A | 16 buckets initiaux |
| Configuration B | 1 024 buckets initiaux |
| Autres paramètres | Identiques : FNV-1a, chaînage, seuil de redimensionnement de 0,75, agrandissement ×2, journalisation et authentification |

Les données sont générées dans l’ordre : aucune graine aléatoire n’est nécessaire. Les générer avant de démarrer le chronomètre. Comparer les configurations sur la même machine, avec les mêmes versions du serveur et du client et le même mode de lancement.

Nous comparons d’abord le nombre initial de buckets, car le paramètre `CACHE_BUCKET_COUNT` existe déjà. Il peut influencer le chargement et les redimensionnements ; l’agrandissement automatique peut ensuite conduire les deux configurations à des capacités similaires.

## 2. Déroulement

Pour chaque taille de jeu de données et chaque configuration :

1. Définir `CACHE_BUCKET_COUNT` à 16 ou 1024 et redémarrer le serveur avec un cache vide.
2. Générer les N paires clé/valeur avant la mesure.
3. Démarrer un chronomètre, envoyer N requêtes POST, lire entièrement leurs réponses, puis arrêter le chronomètre. Chaque insertion doit renvoyer 201.
4. Lire chaque clé insérée une fois et vérifier sa valeur exacte. Effectuer cette vérification hors chronométrage. Arrêter l’essai et signaler un échec si une valeur est incorrecte ou absente.
5. Démarrer un nouveau chronomètre, envoyer N requêtes GET pour ces clés dans le même ordre, lire entièrement leurs réponses, puis arrêter le chronomètre. Chaque lecture doit renvoyer 200.
6. Enregistrer les deux durées et le nombre d’erreurs dans le rapport.

Les lectures mesurées ont lieu après la vérification, qui a déjà sollicité le cache. Chaque essai comprend N insertions mesurées, N lectures mesurées et N lectures de vérification non chronométrées : soit 3N requêtes HTTP au total.

Répéter chaque cas trois fois, en redémarrant le serveur à chaque essai. Conserver tous les résultats et utiliser la médiane des trois durées pour comparer : trier les trois temps et retenir celui du milieu. Si les écarts varient beaucoup entre les essais, indiquer qu’aucune configuration ne se démarque clairement.

## 3. Mesures à conserver

- Temps total d’insertion et temps total de lecture, en millisecondes.
- Temps moyen par opération côté client : durée de la phase / N. Ce temps inclut HTTP et le travail du client ; il ne correspond pas au seul hachage.
- Opérations réussies, réponses inattendues, erreurs de connexion et valeurs incorrectes. Un essai en échec ne permet pas de comparer valablement les vitesses.
- Date, commit Git, version de Go, machine/système, mode de lancement et paramètres d’authentification et de journalisation. Ne jamais enregistrer les credentials.
- Journalisation : l’événement `logging configured` écrit au démarrage et le volume de journaux de l’essai, c’est-à-dire le nombre d’événements `http request` (par exemple `grep -c '"msg":"http request"'` sur la sortie du serveur). Garder la même configuration pour A et B : une différence de journalisation ne doit pas être attribuée au moteur.
- Métriques : enregistrer `/metrics` (avec l’en-tête Bearer) avant et après chaque phase chronométrée, par exemple avec `curl ... > metrics-avant.txt`. La différence des compteurs donne les requêtes, hits, misses et erreurs internes de la phase. L’histogramme `cachelab_http_request_duration_seconds` mesure la latence côté serveur ; le chronomètre du client mesure la latence côté client, qui inclut HTTP et le réseau : ne pas les mélanger. Lire `/metrics` le même nombre de fois pour A et B, car chaque lecture provoque une courte pause du programme.

Utiliser le [modèle de rapport](performance-report-template.md). Mesurer les performances sans `-race` ; exécuter les tests de correction et de détection des courses séparément. Utiliser un binaire serveur déjà compilé pour exclure la compilation et le démarrage des mesures.

Une fois SEC-01 intégré, activer l’authentification pour les essais finaux. En attendant, identifier les résultats comme exploratoires. Garder le même réglage d’authentification pour A et B.

## 4. Interprétation

Comparer la même opération et la même taille de jeu de données. Une configuration peut insérer plus vite sans lire plus vite. Expliquer ce compromis plutôt que de désigner un gagnant global.

Cette première expérience ne mesure pas le débit maximal, le P99, la RAM, le coût du hachage ou la complexité Big O. La concurrence, les volumes plus importants, la comparaison des seuils de charge et les diagnostics internes pourront être ajoutés ensuite.

Valider ces paramètres simplifiés avec l’équipe avant de les utiliser comme périmètre final du ticket.
