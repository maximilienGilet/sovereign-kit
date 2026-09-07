# Provisioning animé et récupération après erreur

Date : 2026-09-04
Statut : implémentée, revue indépendamment et vérifiée le 2026-09-04. Modifications conservées sans commit.

## Objectif et périmètre

Donner une présence visuelle forte aux attentes du TUI sans simuler l’avancement, et permettre de détruire explicitement une instance créée par la session après une erreur.

Le motif, les étapes réelles, la confirmation de destruction et le comportement compact ont été approuvés. L’ancienne instance 49798002 a déjà été détruite par l’utilisateur : aucune opération ne doit la cibler durant le développement.

Hors périmètre : nouvelle location automatique, reprise d’une instance extérieure à la session, destruction automatique, gestion générale du parc Vast, refonte des écrans de sélection.

## Présentation

### Grand écran de provisioning

Dans le cadre plein écran existant, afficher une sphère stylisée en caractères de terminal : anneaux concentriques fins, cyan, un point lumineux et une traînée bleu sombre. Les anneaux pulsent lentement du centre vers l’extérieur. Le motif garde des dimensions stables ; aucune vibration du texte adjacent.

```text
                  · · · · ·
             ·  ╭───────────╮  ·
          ·  ╭──╯           ╰──╮  ·
         ·  │    ╭─────────╮    │  ·
        ·   │   │   ◇   ◇   │   │   ·
         ·  │    ╰─────────╯    │  ·
          ·  ╰──╮           ╭──╯  ·
             ·  ╰───────────╯  ·
                  · · · · ·

          WAITING FOR YOUR INSTANCE
                     00:42
```

Sous le motif : libellé de l’opération, durée écoulée de cette opération et étapes effectivement terminées. Une onde brève signale un changement réel d’étape. Pas de pourcentage estimé, pas de journaux défilants, pas de comparaison entre offres ou recettes.

Le libellé de l’étape en cours porte un effet de shimmer : un reflet étroit se déplace lentement de gauche à droite, du bleu-cyan au blanc puis au cyan, avec une pause entre les passages. Seule l’étape active est animée ; les étapes terminées restent fixes avec leur coche, les étapes futures restent atténuées. Ne pas dupliquer ce libellé dans plusieurs zones animées. La largeur du texte et sa position restent constantes, et le texte demeure lisible sur toute la durée du passage. Le shimmer utilise la même horloge d’animation que le motif, sans boucle de ticks indépendante. Il s’arrête sur une erreur, une confirmation ou une sortie de l’écran. Sans couleur ou en mode accessible, le marqueur textuel de l’étape active suffit ; aucun shimmer.

Le motif utilise les caractères Unicode déjà adaptés au TUI. Le mode accessible reste textuel, sans animation ni séquences de réécriture.

### Avancement fidèle

Les événements du backend, jamais les frames d’animation, pilotent les étapes : création, attente de la machine, vérification des empreintes SSH, lancement du serveur, sauvegarde et vérification de la connexion.

Une coche indique uniquement une opération réellement terminée. La sauvegarde seule ne signifie pas « serveur prêt » : conserver l’état actuel « configuration sauvegardée, route non vérifiée ». Le motif final autour d’un ✓ et la transition vers le dashboard sont réservés à la connexion réellement validée. Aucune animation finale ne doit retarder les interactions.

Les demandes d’intervention, notamment l’approbation des empreintes SSH, remplacent le loader par le formulaire habituel. Le loader reprend seulement lorsque le travail reprend.

L’identifiant de l’instance et l’avertissement de facturation restent visibles dès que l’instance est créée. Ctrl+C conserve le comportement de confirmation et d’arrêt local ; il ne détruit rien implicitement.

### Autres attentes et petits terminaux

Réutiliser un spinner Charm Bubbles compact pour la recherche d’offres, les recherches/inspections Hugging Face et les attentes qui ne nécessitent pas le grand écran. Le libellé correspond à l’opération réelle ; l’animation ne bloque ni la navigation ni l’annulation.

Réduire ou retirer le motif lorsque l’espace manque, avant de tronquer une information essentielle. À 30×10, prioriser l’opération, l’identifiant, l’avertissement et les commandes. Les détails longs peuvent défiler dans le viewport existant. Sous la taille minimale déjà prise en charge, garder le diagnostic de redimensionnement.

## Gestion des erreurs et destruction

À l’erreur, arrêter l’animation et afficher le motif figé avec un ! ambre lorsque l’espace le permet. Conserver l’erreur lisible et son identifiant d’instance.

Proposer « Détruire l’instance #ID » seulement pour l’identifiant positif fourni par la création de la session courante. Ne jamais déduire cet identifiant d’un texte d’erreur, d’un identifiant d’offre, ni d’une configuration sans lien avec cette session.

Flux : erreur → action détruire → confirmation explicite → destruction en cours → vérification → destruction confirmée ou échec/incertitude.

- Confirmation avec l’identifiant et la perte des données de cette instance ; Annuler est sélectionné par défaut.
- Avant destruction, arrêter et attendre la fin des opérations de provisioning/connexion pouvant encore agir sur cette instance.
- Une confirmation déclenche une seule demande ; les appuis répétés ne lancent pas de demandes concurrentes.
- Utiliser un contexte dédié et borné : le contexte du provisioning ayant échoué peut déjà être annulé.
- Afficher un loader pendant la demande et sa vérification. Un simple acquittement de requête ne suffit pas à annoncer une destruction confirmée.
- La vérification utilise le contrat documenté de Vast. Une erreur réseau, d’authentification ou une réponse ambiguë n’est pas une preuve de destruction. Vérifier le contrat officiel avant de coder cette partie.
- Contrat vérifié : `DELETE /api/v0/instances/{id}` exige une réponse `200` avec `success:true`. La vérification ciblée `GET /api/v0/instances/{id}/?owner=me` exige un document JSON unique et complet ; seul `instances:null` établit l’absence. Un champ manquant ou un simple HTTP 404 n’est pas une confirmation. Sources : [référence Vast](https://docs.vast.ai/api-reference/instances/destroy-instance), [SDK officiel](https://github.com/vast-ai/vast-cli/blob/master/vastai/api/instances.py).
- En cas d’échec ou de délai dépassé, conserver l’identifiant, la prudence sur la facturation et l’action Réessayer. Après une issue ambiguë, vérifier d’abord l’état avant une nouvelle demande.
- Après confirmation, afficher « Instance #ID détruite », enlever l’avertissement de ressource potentiellement active et empêcher la réutilisation de la connexion détruite. Ne pas annoncer le remboursement ou l’absence de frais déjà engagés.
- Quitter reste possible. Si la destruction n’est pas confirmée, expliquer que quitter ne garantit pas sa réalisation.

Une configuration sauvegardée visant cette instance ne doit pas être présentée comme utilisable après destruction. Préserver les fichiers non liés ; signaler la configuration concernée sans effacement général implicite.

En mode accessible, fournir les mêmes décisions et confirmations en texte, sans animation. Une entrée vide ou EOF ne confirme jamais la destruction.

## Correction de l’attente Vast

Aujourd’hui, `waitForRunning` rejette immédiatement `actual_status == ""`. Traiter le statut vide ou composé d’espaces comme un état transitoire, au même titre que les états de démarrage reconnus, dans la limite du délai existant.

Ne pas déclarer une instance prête sans le statut `running` et les coordonnées SSH valides exigées aujourd’hui. Conserver les erreurs explicites pour `offline`, `exited` et `unknown`. Ne pas masquer une panne du fournisseur derrière un loader infini.

## Intégration

- Conserver un seul programme Bubble Tea et les enfants intégrés existants.
- Isoler le rendu/animation du provisioning dans un petit modèle réutilisable ; utiliser Bubble Tea pour les ticks, Bubbles pour les spinners et Lip Gloss pour la mise en forme.
- Les événements `setup.Progress` alimentent l’état réel. Ajouter uniquement les événements nécessaires aux opérations actuellement non nommées.
- Protéger les ticks et résultats asynchrones avec les générations de session/écran ; ignorer les résultats périmés. Pas de chaîne de ticks active après sortie de l’écran, erreur, annulation ou transfert au client externe.
- Exposer la destruction via une dépendance étroite et injectable, séparée de la recherche/location. Ne pas ajouter une suppression automatique à l’orchestrateur.
- Garder les secrets hors des messages d’UI et appliquer la rédaction existante aux erreurs de destruction.

## Validation attendue

Tests déterministes avant chaque modification de comportement :

1. Statut vide puis loading/running : attente réussie ; statut vide persistant : timeout ; offline : erreur ; annulation : arrêt.
2. Animation et shimmer actifs seulement pendant le travail et sur l’étape courante, événements périmés ignorés, largeur du libellé constante, étapes et durée cohérentes, aucun faux état « prêt ».
3. Confirmation de destruction annulée ou EOF : zéro appel ; confirmation acceptée : une seule cible exacte et un seul appel concurrent.
4. Acquittement, vérification différée, panne réseau, refus d’authentification, timeout et retry : avertissements conservés jusqu’à preuve de destruction.
5. Arrêt du worker avant destruction, contexte indépendant, aucune création supplémentaire et aucune destruction automatique.
6. Mise en page aux tailles 30×10, 72×24, 108×30, 150×34 et grand terminal ; mode accessible sans animation.

Vérifier ensuite la suite Go, les courses sur les packages modifiés et une session TUI sous terminal avec dépendances fictives. Aucun appel réel de location ou de destruction pendant les tests.

## Résultats

- Suite complète : 447 tests passés, 14 packages.
- Détection des courses : 309 tests passés sur CLI, setup et Vast.
- Analyse statique globale et vérification des diffs : propres.
- Revue finale : avertissement/identifiant épinglés pendant l’approbation SSH, étapes futures atténuées et arrêt complet de connexion avant destruction vérifiés.
- Essais PTY simulés : 150×34 en couleur (shimmer, pause/reprise, erreur figée, destruction confirmée), 30×10 (spinner, messages et confirmations visibles), restauration du terminal à la sortie.
- Aucune instance réelle créée ou détruite. Le comportement d’un fournisseur réel n’est pas validé par ces simulations.
