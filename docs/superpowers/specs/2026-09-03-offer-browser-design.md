# Sovereign Kit — sélecteur visuel d’offres

## Direction approuvée

Remplacer le radar par un sélecteur d’offres utile, visuel et entièrement TUI.
Conserver Bubble Tea, Bubbles, Huh et Lip Gloss, la session unique et les garanties
du setup existant. Aucune interface web ni nouvelle dépendance graphique.

Hypothèse retenue après « on peut faire ça » : plusieurs pays peuvent être cochés,
avec « Tous les pays » par défaut. Leur union constitue le filtre, pas leur
intersection. Le choix de recette reste indépendant de la comparaison d’offres.

## Parcours

Fournisseur → recette / modèle custom → accès fournisseur → navigateur d’offres
→ examen de l’offre choisie → confirmation payante → étapes existantes.

Toutes les recettes passent par un choix explicite d’offre, y compris les recettes
intégrées. La première ligne peut recevoir le focus, sans autoriser de location.
Entrée dans le navigateur ouvre l’examen ; seule la confirmation payante distincte
peut déclencher la création. Aucun nouvel essai automatique de création.

## Écran principal

- Bandeau : recette choisie, filtres pays, tri actif, nombre d’offres chargées.
- Liste à gauche : code pays, GPU, prix horaire, fiabilité déclarée par Vast.
  Navigation au clavier, sélection clairement visible même sans couleurs.
- Fiche à droite : prix horaire et estimation mensuelle (730 heures de calcul),
  pays / région, matériel, mémoire, disque et réseau de l’offre sélectionnée.
- Barres graduées : VRAM disponible par GPU face au minimum requis ; disque
  disponible par instance face à l’allocation demandée. Valeurs exactes et unités
  visibles ; marqueur du besoin. Aucune assimilation à un taux d’utilisation.
  Inconnu signifie inconnu, jamais zéro. Pas de score composite ni radar.
- Compatibilité GPU / nombre : texte « exact » ou « préférence », selon la
  politique réelle de la recette ; pas de faux score de performance.
- Inspection i : identifiants, modèle / révision, runtime, provenance et détails
  complets. Ces données ne concurrencent plus les informations de décision.
- Pied de page fixe : ↑↓ naviguer, f pays, s tri, r actualiser, i détails,
  Entrée examiner, Échap retour. Adapter l’aide à la place et à l’écran actif.

L’effet visuel vient d’une hiérarchie nette, de blocs alignés et d’une transition
brève des barres lors du changement de sélection. Les valeurs exactes changent
immédiatement. Les animations ne bloquent jamais une touche, ne tournent pas en
boucle et ne simulent pas une vérification réseau.

Sur petit terminal, la liste et la fiche deviennent deux vues successives du
même écran. Le filtre et les actions restent accessibles ; les détails défilent.
Sous 30×10, aucune action invisible ne peut être validée. Sur grand terminal,
limiter les colonnes de texte pour garder une bonne lisibilité.

## Pays et tri

Le filtre ouvre un choix multiple recherchable par nom ou code pays. Les choix
ne dépendent pas seulement des pays présents dans les premiers résultats.
Codes ISO à deux lettres normalisés, sans doublon ; liste vide = tous pays.
Échap annule les modifications, Appliquer les valide et lance une recherche.
Aucun raccourci géographique implicite du type « Europe » dans cette version.

La recherche fournisseur reçoit le filtre pays et le tri avant la limitation
des résultats :

- geolocation: {"in": ["FR", "DE", "NL"]}, omis pour tous pays ;
- prix : dph_total ascendant, tri par défaut ;
- fiabilité : reliability descendante, puis prix ascendant.

Charger jusqu’à 100 offres par recherche, plutôt que cinq. Afficher le nombre
chargé, sans prétendre connaître le total du marché. Si 100 sont retournées,
signaler la limite atteinte et inviter à affiner. Pas de pagination inventée.

Changer le filtre ou le tri lance une nouvelle recherche annulable. Les anciens
résultats ne sont pas sélectionnables pendant son chargement. Un résultat tardif
ne peut pas remplacer la recherche courante. Garder l’identifiant sélectionné
uniquement s’il figure encore dans le nouveau résultat éligible.

Le pays affiché provient de la localisation déclarée par Vast, pas d’une
géolocalisation vérifiée par Sovereign Kit. Une localisation inexploitable est
affichée comme inconnue ; ne pas l’associer arbitrairement à un pays. Conserver
la chaîne brute dans l’inspection.

Sans résultat, garder filtres et contrôles actifs ; proposer leur modification
ou une actualisation. Ne pas basculer silencieusement vers tous les pays.
Une erreur réseau n’efface pas les filtres et n’autorise pas une ancienne offre.

Source vérifiée : https://docs.vast.ai/api-reference/search/search-offers
(geolocation accepte une liste de codes pays ; opérateurs in et ordre de tri).

## Examen et sécurité

Retirer aussi le radar de la confirmation existante. Réutiliser les faits et
barres de l’offre retenue, sans seconde sélection concurrente ni contrôles
dupliqués. Conserver les détails consultables et les avertissements de coût :
facturation immédiate, calcul seul, stockage / trafic sortant / taxes exclus.
« Annuler » reste le choix par défaut.

Le montant est celui de l’offre retournée par le fournisseur, pas un devis
garanti ni une réservation. Une actualisation invalide la confirmation précédente.
Si l’offre n’est plus disponible au moment de créer, afficher l’échec et ne
substituer aucune autre offre. Garder la protection existante contre les créations
répétées ou incertaines et conserver tout identifiant d’instance obtenu.

Préserver les contraintes matérielles et d’éligibilité existantes, la confirmation
de remplacement avant création, les validations d’identité et d’empreinte SSH,
la route loopback et l’absence de destruction distante.

## Responsabilités techniques

- Client Vast : validation / normalisation du filtre et du tri, sérialisation
  de la recherche, normalisation de la réponse. Aucun rendu.
- Service de recherche : contraintes de recette et éligibilité partagées avec
  le provisionnement. Pas de copie divergente du filtrage matériel.
- Écran d’offres dédié : liste, filtres, fiche, inspection, dimensions ; il émet
  des intentions de recherche et de sélection, sans posséder de clé API.
- Adaptateur CLI / racine : recherche asynchrone, annulation et générations,
  transfert de l’offre choisie au pipeline existant. Une seule session terminal.
- Examen : rendu commun des faits, confirmation séparée, aucune recherche ou
  création déclenchée par la seule sélection d’une ligne.

Le mode accessible permet également pays multiples, choix d’ordre et sélection
explicite, sous forme de questions et d’une liste numérotée. Les confirmations,
les entrées redirigées, EOF et la protection des secrets restent inchangés.

## Validation

- Payloads HTTP contrôlés : pays multiples / tous pays / codes invalides,
  ordres de tri, limite et contraintes matérielles conservées.
- Recettes intégrées et custom : sélection explicite ; aucun appel de création
  pendant navigation, filtrage, inspection ou examen simple.
- Filtres appliqués au serveur, pas uniquement aux résultats déjà chargés.
- Recherche vide, erreur, annulation, réponses tardives, disparition d’une ligne,
  changement de tri et conservation conditionnelle de la sélection.
- Barres exactes, valeurs inconnues, GPU multiples, noms longs, prix et fiabilité
  absents : pas de valeur de substitution trompeuse.
- Clavier / couleur désactivée, retour depuis l’inspection, contrôles fixes,
  redimensionnement et tailles 30×10, 72×24, 108×30, 150×34, 320×80.
- Mode texte, refus / EOF, double validation, création incertaine et avertissements
  post-création toujours couverts.
- Tests et rendu terminal avec offres fictives uniquement ; aucune location
  réelle pour valider l’interface.

## État de conception

- [x] Contexte et limites actuelles inspectés.
- [x] Approche liste + fiche proposée et approuvée dans la conversation.
- [x] Pays multiples retenus comme hypothèse explicite.
- [x] Contrat géographique fournisseur vérifié.
- [x] Spécification relue : pas de comparaison de recettes, pas de score inventé,
  pas de choix automatique ni de création implicite.
- [x] Validation de cette spécification écrite par l’utilisateur (« review et implemente »).
- [x] Plan d’implémentation puis réalisation et vérification : 385 tests, 247 tests avec détection de concurrence, review indépendante et parcours TUI avec fournisseur fictif.
