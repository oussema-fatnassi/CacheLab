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


## 5. Extension PERF-01-B — Trois moteurs

Version **B.1 — 14 novembre 2026** · [Ticket CL-141](https://oussemafatnassi.atlassian.net/browse/CL-141).

Les sections précédentes conservent l’expérience A (16/1 024 buckets) et les options du client existant. Pour la comparaison des moteurs, appliquer les paramètres ci-dessous : ils remplacent les réglages temporels de l’étape A, sans modifier ses résultats historiques.

| Paramètre | Comparaison B.1 |
| --- | --- |
| Moteurs et volumes | `chained`, `recursive`, `go-map` ; 1 000, 10 000, 100 000 et 1 000 000 clés |
| Données | Clés ASCII de 24 octets (`bench-` + indice sur 18 chiffres), valeurs de 100 caractères `x`, remplacés par `y` après modification ; graine 42 |
| Opérations | N par phase ; lectures uniformes avec remise, créations distinctes dans l’ordre |
| Concurrence et débit | 1 worker, sans débit imposé ; série P99 séparée à 8 workers |
| Répétitions | 3 ; ordre chained/recursive/go-map, puis recursive/go-map/chained, puis go-map/chained/recursive |
| Échauffement | N lectures présentes hors mesure ; 0 avant la création à froid |

Utiliser le même poste, le même binaire serveur/image et le même client, avec **un moteur mesuré à la fois**. Garder contrat CRUD, `sync.Mutex`, limites, authentification, logs et métriques identiques. Create refuse un doublon ; Update ne crée pas une clé absente. Fixer FNV-1a pour les deux moteurs custom ; Go map conserve son hash interne.

Réglages : chained commence à 16 buckets, seuil 0,75, croissance ×2 ; recursive utilise 16 branches, profondeur maximale 16 et seuil de subdivision consigné ; Go map ne reçoit pas de capacité explicite. Noter les fonctionnalités actives : TTL/éviction/budget mémoire absents ou désactivés pour le socle initial, puis réglages communs pour la campagne finale.

### Déroulement complémentaire

Démarrer à vide et générer les données hors mesure. Mesurer N créations, vérifier les N valeurs, effectuer N lectures d’échauffement, puis mesurer séparément N lectures présentes, N lectures absentes, N modifications et N suppressions. Vérifier les valeurs après modification et l’absence après suppression, hors chronométrage. Pour une opération indépendante, préparer un nouveau jeu équivalent de N clés avant vérification et échauffement. Les absences utilisent `miss--` au lieu de `bench-`.

Ajouter deux profils de **N opérations au total**, après préchargement, vérification et N lectures d’échauffement :

| Profil | Création | Lecture présente | Lecture absente | Modification | Suppression |
| --- | --- | --- | --- | --- | --- |
| Équilibré | 20 % | 30 % | 10 % | 20 % | 20 % |
| Écritures majoritaires | 30 % | 10 % | 0 % | 30 % | 30 % |

Par cycle de 10 : le profil équilibré crée 2 clés, lit 3 présentes et 1 absente, modifie 2 préchargées puis supprime les 2 nouvelles ; le profil écritures crée 3 clés, les modifie, les supprime puis lit 1 préchargée. Réserver des clés distinctes aux lectures et modifications du jeu initial. Les nouvelles clés (`new---` + indice sur 18 chiffres) sont uniques par cycle. Si une série concurrente est ajoutée, chaque worker possède ses clés modifiables et respecte cet ordre. Vérifier l’état final hors mesure.

### Mesures et limites

- Séparer benchmarks directs (N opérations, ns/op, B/op, allocs/op) et HTTP (latences, débit, erreurs). Préparation, préchargement, vérification et échauffement sont hors mesure ; Create mesure son propre chargement. Tester avec `-race`, mesurer sans `-race`.
- Relever CPU et mémoire du serveur/client à vide, après chargement, après échauffement, avant/après les phases et après suppression. Même outil et mêmes collectes `/metrics`, hors chronométrage. Conserver deltas CPU, durée de phase et allocations directes. Ne pas additionner HeapAlloc, Sys et RSS ni les assimiler à la taille du cache ; N × 124 octets ne représente que les données. Garder le GC normal et consigner les réglages Go/Docker.
- Cible locale : **P99 GET présent < 10 ms**, pour chaque moteur/N, avec 8 workers, N clés préchargées et vérifiées, N lectures d’échauffement puis N lectures présentes mesurées, sans débit imposé. Les trois essais doivent réussir sans erreur et respecter la cible. Signaler la saturation du client ; tout niveau non exécuté reste non vérifié.
- Comparer au moins deux hashes 64 bits documentés dans une campagne séparée, sur les moteurs custom uniquement : même structure et paramètres, seul le hash change, trois répétitions en alternant les hashes. Séparer hash seul, moteur direct et HTTP.
- Conserver toutes les répétitions, médiane/min/max, données brutes, versions serveur/client, configuration et limites dans le modèle de rapport. Séparer les machines et signaler toute mesure manquante. L’analyse définit n, L, charge, profondeur d et candidats terminaux c ; distinguer moyenne, pire cas et amorti, avec collisions, croissance et sources Go. Une courbe ne prouve pas O(1).

Les campagnes sont bornées en opérations : **aucune durée minimale ni endurance**. Les timeouts techniques et règles de TTL restent applicables. Versionner tout changement avant les essais et identifier les fonctions ajoutées pour la campagne finale.

Ce ticket prépare le protocole : l’extension du client relève de [QA-02-B / CL-146](https://oussemafatnassi.atlassian.net/browse/CL-146), la sélection des moteurs et les tests des vrais serveurs/redémarrages de [CL-158](https://oussemafatnassi.atlassian.net/browse/CL-158). Le client actuel ne réalise pas encore toute l’étape B.

Sources : [protocole Confluence](https://oussemafatnassi.atlassian.net/wiki/pages/viewpage.action?pageId=26804225) et [modèle Confluence](https://oussemafatnassi.atlassian.net/wiki/pages/viewpage.action?pageId=26738690), versions 5.
