# PERF-01-A — Comparaison simple des performances

Objectif : charger des données, mesurer les temps d’insertion et de lecture, puis comparer deux configurations de notre table de hachage actuelle.

Ce document remplace le précédent brouillon détaillé. Le client QA-02-A est disponible avec `make loadtest ARGS="-count 1000 -label buckets-16"`. Les rapports créés par cette commande contiennent les mesures réellement effectuées.

## 1. Paramètres de l’expérience

| Paramètre | Valeur |
| --- | --- |
| Tailles des jeux de données | 1 000, 10 000 et 100 000 clés |
| Clés | `bench-0`, `bench-1`, etc. |
| Valeurs | La même chaîne de 100 caractères ASCII `x` pour chaque clé |
| Clients | Un client séquentiel par défaut ; concurrence configurable |
| Sélection des lectures | Tirages reproductibles avec remise, graine 42 ; les clés sont insérées dans l’ordre |
| Débit / échauffement / durée / misses | Par défaut : sans limitation, 0 s, N lectures, 0 % de misses |
| Configuration A | 16 buckets initiaux |
| Configuration B | 1 024 buckets initiaux |
| Autres paramètres | Identiques : FNV-1a, chaînage, seuil de redimensionnement de 0,75, agrandissement ×2, journalisation et authentification |

Les données sont générées dans l’ordre avant de démarrer le chronomètre. La graine détermine la sélection des lectures, pas le contenu des clés. Comparer les configurations sur la même machine, avec les mêmes versions du serveur et du client et le même mode de lancement.

Nous comparons d’abord le nombre initial de buckets, car le paramètre `CACHE_BUCKET_COUNT` existe déjà. Il peut influencer le chargement et les redimensionnements ; l’agrandissement automatique peut ensuite conduire les deux configurations à des capacités similaires.

## 2. Déroulement

Pour chaque taille de jeu de données et chaque configuration :

1. Définir `CACHE_BUCKET_COUNT` à 16 ou 1024 et redémarrer le serveur avec un cache vide.
2. Générer les N paires clé/valeur avant la mesure.
3. Démarrer un chronomètre, envoyer N requêtes POST, lire entièrement leurs réponses, puis arrêter le chronomètre. Chaque insertion doit renvoyer 201.
4. Lire chaque clé insérée une fois et vérifier sa valeur exacte. Effectuer cette vérification hors chronométrage. Arrêter l’essai et signaler un échec si une valeur est incorrecte ou absente.
5. Si un échauffement est configuré, effectuer ces lectures préparatoires sans les ajouter aux mesures. Puis démarrer un nouveau chronomètre et envoyer N GET, ou envoyer pendant la durée configurée. La graine reproduit les tirages de clés avec remise. Attendre la fin des réponses déjà envoyées après l’arrêt des envois. Attendre 200 pour une clé présente et 404 pour un miss volontaire.
6. Enregistrer les deux durées et le nombre d’erreurs dans le rapport.

Les lectures mesurées ont lieu après la vérification, qui a déjà sollicité le cache. Le client contrôle aussi les valeurs des lectures mesurées ; les temps de phase incluent ce travail côté client. Avec les options par défaut, chaque essai comprend 3N + 4 requêtes HTTP. Avec durée ou échauffement, consulter les compteurs de chaque phase pour connaître le nombre réel.

Répéter chaque cas trois fois, en redémarrant le serveur à chaque essai. Conserver tous les résultats et utiliser la médiane des trois durées pour comparer : trier les trois temps et retenir celui du milieu. Si les écarts varient beaucoup entre les essais, indiquer qu’aucune configuration ne se démarque clairement.

## 3. Mesures à conserver

- Temps total d’insertion et temps total de lecture, en millisecondes.
- Temps moyen et P50/P95/P99 des latences individuelles : du départ de la requête à la lecture complète de la réponse, avant validation JSON. Ces temps incluent HTTP et le réseau, pas le seul hachage. La durée totale de phase inclut aussi le travail du client, l’ordonnancement et l’attente finale.
- Débit proposé, débit réellement réussi, statuts, timeouts et erreurs de transport. Séparer `GET_hit` et `GET_miss` ; ne pas assimiler un 404 volontaire à un échec.
- Si un débit est imposé, conserver les créneaux non envoyés et les retards d’envoi. Une saturation ne doit pas être présentée comme un débit tenu. Les percentiles au rang le plus proche portent sur les réponses correctes ; ne pas moyenner les percentiles des répétitions.
- Opérations réussies, réponses inattendues, erreurs de connexion et valeurs incorrectes. Un essai en échec ne permet pas de comparer valablement les vitesses.
- Date, commit Git, version de Go, machine/système, mode de lancement et paramètres d’authentification et de journalisation. Ne jamais enregistrer les credentials.
- Journalisation : l’événement `logging configured` écrit au démarrage et le volume de journaux de l’essai, c’est-à-dire le nombre d’événements `http request` (par exemple `grep -c '"msg":"http request"'` sur la sortie du serveur). Garder la même configuration pour A et B : une différence de journalisation ne doit pas être attribuée au moteur.
- Métriques : le client enregistre automatiquement `/metrics` avec l’en-tête Bearer avant et après chaque phase chronométrée. Les quatre exports et leurs dates figurent dans le champ `metrics` du rapport JSON. Comparer les compteurs des routes CRUD concernées : les collectes elles-mêmes peuvent aussi augmenter les compteurs globaux. L’histogramme `cachelab_http_request_duration_seconds` mesure la latence côté serveur ; les mesures du client incluent HTTP et le réseau. Lire `/metrics` le même nombre de fois pour A et B, car chaque lecture provoque une courte pause du programme.

Utiliser le [modèle de rapport](performance-report-template.md). Mesurer les performances sans `-race` ; exécuter les tests de correction et de détection des courses séparément. Utiliser un binaire serveur déjà compilé pour exclure la compilation et le démarrage des mesures.

Une fois SEC-01 intégré, activer l’authentification pour les essais finaux. En attendant, identifier les résultats comme exploratoires. Garder le même réglage d’authentification pour A et B.

## 4. Interprétation

Comparer la même opération et la même taille de jeu de données. Une configuration peut insérer plus vite sans lire plus vite. Expliquer ce compromis plutôt que de désigner un gagnant global.

Le scénario avancé et ses options sont décrits dans le README. Le test simple reste le point de départ ; n’activer concurrence, débit, échauffement et durée qu’en conservant les mêmes paramètres entre configurations. Les exports donnent des mesures mémoire du processus, pas le coût exact du cache. Aucun résultat ne prouve à lui seul une complexité Big O.

Valider ces paramètres simplifiés avec l’équipe avant de les utiliser comme périmètre final du ticket.
