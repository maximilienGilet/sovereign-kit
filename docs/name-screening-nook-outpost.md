# Criblage préliminaire de noms — **Nook** et **Outpost**

**Objet.** Première vérification de disponibilité / trouvabilité pour un projet OSS qui installe des environnements privés ou souverains d’agents de code IA.
**Date de consultation :** 1 septembre 2026 (UTC). **Ce document n’est pas une recherche d’antériorités exhaustive, ni un avis juridique ou une clearance de marque.** Une décision de dépôt, de commercialisation ou de changement de nom exige une recherche professionnelle, par territoire et par classes (notamment logiciels/SaaS, sécurité, IA), puis l’analyse du risque de confusion.

## Verdict pratique

| Nom | Disponibilité technique et web | Risque de collision / ambiguïté | GEO et recommandation |
|---|---|---|---|
| **Nook** | Les paquets exacts npm et PyPI existent ; `nook.ai`, `nook.dev` et `nook.com` sont déjà enregistrés. | **Élevé.** Mot commun, produit NOOK de Barnes & Noble et plusieurs projets logiciels visibles. | À éviter comme nom autonome. `Nook AI` / `Nook Private AI` réduit l’ambiguïté mais reste une expression descriptive accolée à un terme déjà très occupé ; prévoir un nom composé réellement distinctif. |
| **Outpost** | Les paquets exacts npm et PyPI existent ; `outpost.ai`, `outpost.dev` et `outpost.com` sont déjà enregistrés. | **Élevé.** Mot commun et emploi direct dans des logiciels, dont les *Outposts* d’Authentik — proche de la sécurité / infrastructure. | À éviter comme nom autonome et plutôt moins favorable que Nook pour ce produit. `Outpost AI` / `Outpost Coding Agents` peut créer une entité éditoriale, mais ne supprime ni la collision d’usage ni le caractère descriptif (poste avancé / déploiement). |

**Choix provisoire :** si l’un des deux doit être conservé pour un prototype interne, préférer **Nook** à **Outpost**, avec un qualificatif constant et un slug distinct (`<marque-distinctive>-nook` plutôt que `nook`). Pour un lancement public, chercher un troisième nom inventé ou un composé arbitraire, puis faire une clearance.

## Constats vérifiables

### Écosystème développeur

| Source officielle | Nook | Outpost | Lecture |
|---|---|---|---|
| [GitHub Search API — `nook` dans le nom](https://api.github.com/search/repositories?q=nook+in%3Aname&per_page=10) | **5 162** résultats renvoyés au moment du relevé. Exemples : [nook-browser/Nook](https://github.com/nook-browser/Nook) (« A new browser ») et [khromov/nook](https://github.com/khromov/nook) (modèles IA locaux dans le navigateur). | [GitHub Search API — `outpost` dans le nom](https://api.github.com/search/repositories?q=outpost+in%3Aname&per_page=10) : **1 737** résultats. Exemples : [hookdeck/outpost](https://github.com/hookdeck/outpost), infrastructure OSS de webhooks sortants, et [vinibaggio/outpost](https://github.com/vinibaggio/outpost). | Ces totaux sont des résultats de recherche par nom, pas une preuve de collision juridique. Ils montrent néanmoins une forte concurrence de référencement et de namespace. |
| [registre npm — `nook`](https://registry.npmjs.org/nook) / [npm — `outpost`](https://registry.npmjs.org/outpost) | `nook` existe, v0.0.2, « Distributed File System ». | `outpost` existe, v1.5.17, agent de gestion/surveillance distante. | Les noms de paquets exacts sont indisponibles sur le registre public npm. Un scope (`@organisation/nook`) est techniquement possible, mais ne résout pas la marque ni la recherche. |
| [PyPI JSON — `nook`](https://pypi.org/pypi/nook/json) / [PyPI JSON — `outpost`](https://pypi.org/pypi/outpost/json) | `nook` existe, v0.1.1, bibliothèque CLI de clés/valeurs. | `outpost` existe, v0.5.2, serveur proxy applicatif. | Les noms de projets exacts sont déjà occupés sur PyPI. |

### Domaines (RDAP)

Le [service RDAP](https://rdap.org/) a retourné des fiches d’enregistrement — donc pas une disponibilité — pour les domaines suivants :

| Nom | `.ai` | `.dev` | `.com` |
|---|---|---|---|
| **Nook** | [nook.ai](https://rdap.org/domain/nook.ai), enregistré le 2020-01-16 | [nook.dev](https://rdap.org/domain/nook.dev), enregistré le 2025-05-15 | [nook.com](https://rdap.org/domain/nook.com), enregistré le 1997-06-30 |
| **Outpost** | [outpost.ai](https://rdap.org/domain/outpost.ai), enregistré le 2018-08-18 | [outpost.dev](https://rdap.org/domain/outpost.dev), enregistré le 2021-05-06 | [outpost.com](https://rdap.org/domain/outpost.com), enregistré le 1997-07-11 |

`rdap.org` n’a pas fourni de service RDAP pour les deux requêtes `.io` durant ce relevé ; aucune conclusion sur `nook.io` ou `outpost.io` ne doit donc en être tirée. Vérifier les TLD et variantes (traits d’union, pluriels, `.org`, `.app`, ccTLD) auprès de leurs registres avant décision.

### Sens concurrents qui affectent la recherche

* **Nook :** [NOOK de Barnes & Noble](https://www.barnesandnoble.com/b/nook/_/N-1pbl) est un emploi commercial majeur lié à la lecture numérique ; le navigateur [Nook](https://github.com/nook-browser/Nook) et un projet GitHub d’IA locale (ci-dessus) renforcent le bruit dans les résultats technologiques. « Nook » est aussi un mot courant anglais (« coin / alcôve »).
* **Outpost :** le terme désigne couramment un avant-poste. Surtout, la documentation officielle d’[Authentik — Outposts](https://docs.goauthentik.io/add-secure-apps/outposts/) désigne des composants de proxy / intégration pour sécuriser des applications : contexte très voisin d’un outil d’infrastructure privée. [Hookdeck Outpost](https://github.com/hookdeck/outpost) ajoute une occurrence OSS d’infrastructure déjà indexée.

## Évaluation GEO / découvrabilité

**Ce qui ne suffira probablement pas.** Le seul mot exact (`Nook` ou `Outpost`) est très ambigu : moteurs classiques, assistants à génération augmentée et corpus de code associeront le terme à des sens installés. La présence de paquets exacts et de nombreux dépôts diminue aussi la capacité à posséder les requêtes de navigation « install nook » / « outpost github ».

**Ce qui peut aider, sans être une clearance.** Employer partout une dénomination stable telle que « *[marque distinctive] Nook — private AI coding-agent environments* » ou « *[marque distinctive] Outpost — sovereign coding agents* », un domaine propre, un dépôt et une organisation GitHub cohérents, plus une page canonique explicitant la différenciation, peut faire émerger une entité dans les réponses génératives. Les associations « private AI », « sovereign AI », « coding agents » sont cohérentes avec le produit mais restent descriptives ; elles ne rendent pas, à elles seules, le nom distinctif ni disponible. Le risque est particulièrement marqué pour **Outpost**, car l’association infrastructure/sécurité existe déjà dans Authentik.

## Marques : limites du présent relevé et prochaine étape

Les portails primaires à couvrir sont [TMview / EUIPO](https://www.tmdn.org/tmview/) (UE et offices participants), [EUIPO eSearch plus](https://euipo.europa.eu/ec2/) et [Data INPI — marques](https://data.inpi.fr/marques) (France). Les interfaces publiques n’ont pas produit de résultat interrogeable depuis cet environnement : TMview charge une application JavaScript sans résultats dans la réponse HTTP ; Data INPI a répondu **HTTP 403** ; EUIPO eSearch a réinitialisé la connexion. **Cela ne signifie ni absence de marque, ni disponibilité.**

Avant toute annonce ou dépôt :

1. rechercher les correspondances exactes, phonétiques, visuelles et conceptuelles dans TMview/EUIPO et INPI, puis les bases nationales des marchés visés ;
2. filtrer au minimum les logiciels, services SaaS, cybersécurité, développement, IA et services cloud, et examiner les oppositions / statuts ;
3. faire examiner les résultats et les usages non enregistrés par un conseil en propriété industrielle / avocat ;
4. choisir ensuite le domaine, les identifiants GitHub/npm/PyPI et déposer la marque si la stratégie le justifie.

## Méthode et traçabilité

Les données de paquets et de dépôts proviennent des API/registre officiels liés dans les tableaux. Les dates et statuts de domaines proviennent des réponses RDAP liées. Les compteurs GitHub et versions de paquets sont des observations ponctuelles au 1 septembre 2026 ; ils changent au fil du temps. Les résultats de marque n’ont pas été inférés à partir de l’échec d’accès aux portails.
