# Sovereign Kit — application terminal unifiée

Date : 2026-09-03

## Intention validée

Faire de `sovkit` une application terminal continue, du setup au dashboard
existant. Supprimer les alternances entre formulaires dans le shell et écrans
plein écran. Réutiliser Bubble Tea, Bubbles, Huh et Lip Gloss ; aucune nouvelle
dépendance graphique. Les commandes directes et le mode accessible restent
disponibles.

Ce document remplace la composition à trois cartes du cockpit décrite dans
`2026-09-03-recipe-hardware-cockpit-design.md`. Ses garanties de véracité,
d'accessibilité et d'absence de comparaison restent applicables.

## Périmètre et entrée

- `sovkit`, sans argument dans un terminal interactif, ouvre l'application.
- Sans configuration : accueil avec action principale « Configurer ».
- Avec configuration valide : accueil « Route configurée, non vérifiée », avec
  actions « Connecter » et « Reconfigurer ». Ne pas ouvrir automatiquement une
  connexion, lancer un client ou louer une instance.
- Configuration invalide : erreur lisible dans l'accueil ; proposer le setup,
  sans écraser le fichier avant validation explicite de son remplacement.
  Cette confirmation s'applique aussi au remplacement d'une configuration valide.
  Dans le parcours Vast, obtenir cet accord avant l'envoi de toute création
  payante, pas seulement avant la sauvegarde finale. Un refus ne loue rien.
- `help`, `--help` et l'appel sans argument hors terminal affichent l'aide.
- `setup` et `start` interactifs entrent dans le même cadre, à leur étape
  respective. Les commandes `catalog`, `dashboard`, `tunnel` et `doctor`
  conservent leur rôle direct et leurs garanties actuelles.
- `ACCESSIBLE`, `TERM=dumb` et les entrées non interactives n'ouvrent pas le
  plein écran. Préserver les parcours textuels existants ; l'appel nu affiche
  l'aide et les commandes disponibles.

## Navigation et cadre permanent

Une seule session Bubble Tea possède le terminal. Un en-tête indique le
parcours et l'étape ; le corps accueille l'écran actif ; un pied de page
réservé affiche les raccourcis réellement disponibles. Pas de menu latéral
global ajouté à la sidebar de recettes.

Parcours Vast : fournisseur → recette → branche Hugging Face si nécessaire →
accès Vast et identité SSH → recherche d'offre → confirmation du coût →
création → validation des clés hôte → configuration → connexion → dashboard.

Parcours manuel : fournisseur → informations SSH → validation et sauvegarde →
connexion → dashboard. Aucun statut « prêt » avant les vérifications réelles.

- Les formulaires Huh deviennent des écrans intégrés, pas des programmes
  autonomes successifs. Les vues recette, confirmation et dashboard suivent
  la même règle.
- Entrée valide l'action locale. Échap ferme d'abord les détails, puis revient
  à l'étape précédente lorsque ce retour est sûr.
- Les saisies et sélections restent en mémoire pendant la session. Modifier
  une étape invalide ses résultats dépendants et confirmations, sans effacer
  arbitrairement les champs indépendants.
- `q` quitte seulement hors saisie textuelle ; Ctrl+C demande l'arrêt avec
  les avertissements adaptés aux opérations déjà engagées.
- Les recherches, vérifications et attentes affichent leur état dans le corps
  de l'application. Les erreurs y restent lisibles avec une action sûre.
- Conserver le mode de sélection d'offre actuel : choix automatique pour les
  recettes intégrées, choix explicite pour le parcours custom. Pas de nouveau
  moteur de recommandation.

## Recettes : une information, un emplacement

Supprimer toute la rangée des trois cartes récapitulatives.

- Le contexte n'apparaît que dans la zone de jauge graduée : valeur exacte,
  dénominateur déclaré, pourcentage et marqueur de cible.
- La sortie maximale apparaît une fois, en ligne près du contexte.
- La concurrence apparaît uniquement dans les slots, avec la légende
  « limite configurée » et le total exact. Ce n'est ni un nombre d'utilisateurs
  garanti ni une performance mesurée. La valeur provient de la recette et est
  transmise au serveur. En vue compacte, remplacer les slots par une ligne
  indiquant cette limite, sans ajouter une seconde occurrence.
- Le bloc matériel conserve GPU, politique exact/préféré, VRAM par GPU et
  disque par instance. Les cas d'usage restent proches de ce bloc.
- La provenance et les détails complets restent dans `i`. Leur répétition
  dans cette vue d'inspection est intentionnelle, contrairement aux doublons
  présents simultanément sur le cockpit.
- Aucun débit inventé ; garder l'indication de débit non mesuré. Le contexte
  natif reste déclaré par la recette, de source inconnue dans le schéma actuel.
- Réutiliser la transition Bubbles existante, sans animation permanente et
  sans retarder la sélection ni les valeurs exactes.

## Accueil Custom Hugging Face

La sélection custom affiche un écran d'introduction, pas un cockpit inconnu :

- Visage évoquant Hugging Face en art ASCII jaune, accompagné du nom textuel.
- Titre invitant à choisir son modèle, et explication courte : rechercher un
  modèle ou saisir `owner/model`, inspecter sa compatibilité, puis renseigner
  les besoins matériels avant toute recherche d'offre.
- Action principale menant à la recherche/saisie existante, intégrée au cadre.
- Aucun slot, jauge vide, GPU fictif ou série de valeurs `UNKNOWN`.
- L'illustration se réduit puis disparaît sur petit terminal ; texte et action
  restent disponibles. Le parcours accessible conserve une version textuelle.
- L'inspection ne garantit ni les performances ni la compatibilité de tout
  modèle Hugging Face. Conserver la classification et les refus actuels.

## Opérations, coûts et sécurité

Avant la création, un retour arrière peut abandonner les résultats de recherche
et impose une nouvelle confirmation si l'offre ou ses paramètres changent.
La création ne part qu'après confirmation explicite du coût. Une pression
répétée sur Entrée ne peut pas déclencher plusieurs créations.

Dès l'envoi de la création, interdire le retour vers une action susceptible de
la rejouer. Une réponse incertaine doit afficher « création à vérifier » ; ne
jamais proposer une relance automatique. Après obtention de l'identifiant,
conserver cet identifiant et l'avertissement de facturation dans l'état de
session, même en cas d'erreur ou d'abandon.

Les erreurs après création proposent de consulter la situation ou de quitter
avec avertissement, pas de recommencer tout le setup. La reprise persistante
après redémarrage et la destruction d'instance sont hors périmètre. Quitter
l'application n'arrête pas une instance payante ; ne jamais suggérer le contraire.

La confirmation des empreintes SSH reste une étape explicite. Ne pas modifier
la validation des clés, la liaison loopback ni le comportement fail-closed.
Clés API masquées, conservées uniquement le temps nécessaire en mémoire ;
aucun secret dans les diagnostics, événements de journalisation, captures ou
journaux. Les messages internes transportant une saisie secrète ne sont jamais
journalisés.

Les opérations asynchrones sont identifiées et annulables quand cela est sûr.
Un résultat tardif ne peut pas remplacer un écran ou choix plus récent.
Les diagnostics sont bornés et expurgés avant affichage, sans écrire directement
dans le terminal possédé par Bubble Tea.

## Connexion et dashboard

Après sauvegarde, afficher le résultat puis une action « Connecter » ; ne pas
confondre configuration enregistrée et route opérationnelle. La connexion
réutilise la création du tunnel et les vérifications existantes.

Le dashboard n'affiche un état opérationnel qu'après vérification. La session
surveille la sortie du tunnel ; une perte retire cet état et bloque le lancement
d'un nouveau client. Une action « Déconnecter » arrête seulement le tunnel local
possédé par cette session et revient à l'accueil.

Pi/OpenCode prennent temporairement le terminal via le mécanisme existant
`tea.ExecProcess`. À leur fermeture, retour au même dashboard et à la même
session. C'est la seule suspension normale du TUI, pas un nouveau setup.
Au retour, actualiser l'état de la route avant d'autoriser un autre lancement.
Quitter arrête les ressources locales possédées par l'application, sans
détruire de ressource distante.

## Découpage technique

- Un modèle racine dédié possède navigation, dimensions, cycle de vie du
  terminal et état de session. Seul ce modèle décide de quitter l'application.
- Les modèles d'écran exposent validation, annulation et retour au parent.
  Leurs adaptateurs autonomes peuvent toujours quitter leur commande directe.
- Les services setup/connexion restent indépendants du rendu. Extraire les
  points de transition nécessaires des fonctions séquentielles existantes,
  sans copier la logique de validation, sélection ou provisionnement.
- Le câblage de production appartient à la couche CLI ; les écrans reçoivent
  des interfaces de service. Éviter un modèle racine monolithique et les cycles
  d'import entre CLI, UI et services.
- Huh gère les champs et leur validation ; Bubbles les listes, vues défilantes,
  aides et animations ; Lip Gloss le cadre. Aucun programme imbriqué ne lit
  directement l'entrée standard pendant la session principale.
- Les ressources réseau et processus ont un propriétaire et un contexte de
  session explicites. Un changement d'écran ne doit pas arrêter le tunnel ni
  laisser une opération orpheline.

## Vérification attendue

Tester les transitions via messages Bubble Tea et services simulés, sans
location réelle : première ouverture, configuration existante/invalide,
parcours manuel/Vast/custom, retour avec conservation des saisies, changement
de recette invalidant les résultats, erreurs et réponses asynchrones tardives.

Tester double validation du coût, création incertaine, abandon après création,
refus du remplacement d'une configuration existante avant toute location,
refus des clés hôte, perte du tunnel, arrêt des ressources locales et retour
d'un client. Aucun de ces scénarios ne doit déclencher une deuxième location.

Vérifier qu'une seule session plein écran traverse le setup et la connexion,
que les écrans enfants ne quittent pas l'application, et que les commandes
directes ainsi que le mode accessible conservent leurs comportements.

Tester les tailles 30×10, 72×24, 108×30, 134×30, 150×34 et 320×80 : contrôles
visibles, formulaires défilants si nécessaire, noms longs et CJK, absence de
doublons de métriques et accueil custom sans fausses mesures. Sous 30 colonnes
ou 10 lignes, afficher une invitation à agrandir sans lancer d'action cachée.

Inspecter les rendus réels, exécuter les tests Go complets, `go vet` et les
vérifications CONTRIBUTING. Ne pas présenter ces tests comme une validation
d'inférence réelle.

## Ordre de réalisation et exclusions

Découper le plan en incréments vérifiables : simplification du cockpit et
accueil custom ; cadre et écrans intégrés ; setup avec confirmations sûres ;
connexion/dashboard et compatibilité des commandes. Livrer un parcours
continu complet, pas seulement un cadre entourant les programmes actuels.

Pas de nouvelle télémétrie, gestion multi-instance, reprise persistante,
suppression distante, comparateur de recettes ni extension des fournisseurs.
Préserver les modifications déjà présentes dans le répertoire de travail.
