# Faits API Vast (offres, états, SSH, facturation)

Fiche issue de la recherche du ticket [#4](https://github.com/maximilienGilet/sovereign-kit/issues/4), contre sources primaires (docs officielles Vast.ai : API reference, guides CLI/SDK, guides instances/billing) et croisée avec l'adaptateur `internal/vast` de la branche `feat/recipe-picker-tui`.

Chaque affirmation est reliée à sa source (URL entre parenthèses). Date de consultation : 2026-09-07. Les mentions **hors docs** signalent des comportements observés dans le code de l'adaptateur (`internal/vast/*.go` sur `feat/recipe-picker-tui`) ou dans des exemples, non documentés dans l'API reference.

---

## 1. Recherche d'offres

### 1.1 Endpoint et modèle de requête

- Endpoint : `POST https://console.vast.ai/api/v0/bundles` ; réponse : objet `{ "offers": [...] }` ([API reference — search offers](https://docs.vast.ai/api-reference/search/search-offers.md), [API Hello World](https://docs.vast.ai/api-reference/hello-world.md)).
- Chaque paramètre de filtre est un **objet opérateur/valeur** : opérateurs `eq`, `neq`, `gt`, `lt`, `gte`, `lte`, `in`, `notin` ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md)).
- Filtres documentés : `limit`, `type`, `verified`, `rentable`, `rented`, `gpu_name`, `reliability`, `num_gpus`, `gpu_ram`, `duration`, `machine_id`, `dlperf_per_dphtotal`, `dph_total`, `flops_per_dphtotal`, `geolocation`, `gpu_arch`, `dlperf`, `cuda_max_good`, `inet_down`, `inet_up`, `inet_down_cost`, `inet_up_cost`, `driver_version`, `compute_cap`, `cpu_arch`, `has_avx`, `cpu_cores`, `cpu_cores_effective`, `cpu_ghz`, `cpu_ram`, `datacenter`, `external`, `disk_bw`, `disk_space`, `bw_nvlink`, `gpu_max_power`, `gpu_max_temp`, `gpu_mem_bw`, `gpu_total_ram`, `gpu_frac`, `gpu_display_active`, `direct_port_count`, `host_id`, `id`, `min_bid`, `mobo_name`, `pci_gen`, `pcie_bw`, `storage_cost`, `static_ip`, `total_flops`, `os_version`, `ubuntu_version`, `verification`, `vms_enabled` ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md)).
- Tri : `order` = liste de paires `[champ, "asc"|"desc"]`, ex. `[["dph_total","asc"]]` ou `[["dlperf_per_dphtotal","desc"]]` ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md), [Hello World](https://docs.vast.ai/api-reference/hello-world.md)).

### 1.2 Modèle de GPU « strict »

- Le filtre strict est l'**égalité exacte** sur `gpu_name` : `{"eq": "<nom canonique>"}` ; l'alternative souple est la liste `{"in": ["RTX_3090", "RTX_4090"]}`. Le nombre de GPU se filtre via `num_gpus` (ex. `{"eq": 1}`) ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md), [Hello World](https://docs.vast.ai/api-reference/hello-world.md)).
- Attention à l'orthographe du nom : l'exemple de l'API reference écrit `"gpu_name": {"eq": "RTX_4090"}` (souligné) tandis que l'exemple Hello World écrit `{"eq": "RTX 4090"}` (espace) ; les payloads de réponse montrent des noms **avec espaces** (`"gpu_name": "RTX A5000"`, `"gpu_name": "RTX 4090"`, exemple d'offre `"gpu_name": "Imaginary RTX 9999"`) ([show instance](https://docs.vast.ai/api-reference/instances/show-instance.md), [show instances](https://docs.vast.ai/api-reference/instances/show-instances.md), [search offers](https://docs.vast.ai/api-reference/search/search-offers.md)). Le CLI documente qu'il faut remplacer les espaces par des soulignés dans les requêtes (`gpu_name=RTX_3090` → `RTX_3090`, « replace spaces with underscores ») ([CLI — search offers](https://docs.vast.ai/cli/reference/search-offers.md)). La valeur exacte à envoyer en REST doit donc être le nom canonique affiché par la marketplace ; l'égalité ne fait pas de correspondance partielle.
- L'adaptateur envoie bien `gpu_name: {"eq": <modèle>}` **uniquement quand** `StrictGPU` est vrai, accompagné de `num_gpus: {"eq": <count>}` (`internal/vast/search.go`), ce qui est conforme au schéma documenté.

### 1.3 Autres champs recherchés par le ticket

- **VRAM** : `gpu_ram` en **Mo** en REST (`{"gte": 24000}`), alors que le CLI exprime les **Go** (« `gpu_ram` in CLI = GB; in REST API = MB (CLI auto-converts) », « GPU RAM in MB ») ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md), note « Key Quirks » incluse dans chaque page OpenAPI). `gpu_total_ram` = RAM GPU totale (toutes GPU) en Mo. L'adaptateur convertit ses Go en Mo (`*1000`) avant envoi (`internal/vast/search.go`) — conforme.
- **Disque** : `disk_space` en **Go** (`{"gte": 146}`) ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md)) ; l'adaptateur l'envoie tel quel (Go) — conforme.
- **Région** : pas de notion de « région » côté serveur : le filtre `geolocation` prend des **codes pays ISO à 2 lettres** (`{"in": ["US", "CA"]}`) ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md)). Les fichiers `internal/vast/regions.go` et `countries.go` de l'adaptateur sont un **mappage local** (CLDR) région→codes pays ; aucun endpoint API Vast dédié aux régions/pays n'existe.
- **Interruptible** : le type d'offre `type` accepte l'enum `ondemand` | `bid` | `reserved` ; **`bid` = interruptible** (« Lower cost but may be interrupted if outbid ») ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md)). Le CLI expose `-t bid` / `--interruptible` / `--bid` ([CLI — search offers](https://docs.vast.ai/cli/reference/search-offers.md)). Sémantique : instance à enchères, priorité basse, « may be paused if outbid or if on-demand requested », reprise automatique quand la priorité revient ([Instance Types](https://docs.vast.ai/guides/instances/choosing/instance-types.md)). L'adaptateur fixe `type: "ondemand"` — conforme à la préférence carte « interruptible interdit par défaut » (aucun cas `bid` implémenté dans `internal/vast`).
- **Prix** : filtre `dph_total` (« Total $/hour rental cost », ex. `{"lte": 0.5}`), tri par `dph_total` ; champ `min_bid` (prix minimal d'enchère $/h) ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md), [CLI — search offers](https://docs.vast.ai/cli/reference/search-offers.md)).

### 1.4 Champs de coût renvoyés par offre

Exemple complet de payload d'offre (`offers[]`) : `id`, `machine_id`, `gpu_name`, `num_gpus`, `gpu_ram` (Mo), `gpu_total_ram`, `cpu_cores_effective`, `cpu_ram` (Mo), `disk_space`, `inet_down`/`inet_up`, `driver_version`, `dph_base`, `dph_total`, `min_bid`, `reliability`, `reliability2`, `geolocation`, `static_ip`, `storage_cost` ($/Go/mois), `inet_down_cost`/`inet_up_cost` ($/Go), sous-objets `search`/`instance` (`gpuCostPerHour`, `diskHour`, `totalHour`, …), `discounted_hourly`, `discount_rate` ([search offers](https://docs.vast.ai/api-reference/search/search-offers.md)). L'adaptateur décode exactement ces champs, gère `dph_total`/`reliability` absents ou `null` (`price_unknown`/`reliability_unknown`) et normalise Mo→Go (`internal/vast/search.go`) — conforme.

---

## 2. Cycle de vie d'une instance

### 2.1 Création

- `PUT https://console.vast.ai/api/v0/asks/{id}` (id = id de l'offre) ; corps : `image` (requis), `disk` (Go), `runtype`, `label`, `env`, `onstart`, `target_state` (`running`|`stopped`), `price` (0.001–128 $/h, instances interruptibles uniquement), `cancel_unavail` (défaut `true` pour on-demand avec `target_state=running`, `false` pour interruptible), `args`, `vm`, … ([create instance](https://docs.vast.ai/api-reference/instances/create-instance.md)).
- `runtype` : enum `ssh` | `jupyter` | `args` | `ssh_proxy` | `ssh_direct` | `jupyter_proxy` | `jupyter_direct` ; défaut `ssh`. L'adaptateur envoie `ssh_direct` ([create instance](https://docs.vast.ai/api-reference/instances/create-instance.md), `internal/vast/client.go`).
- Réponse : `{ "success": true, "new_contract": <id instance>, "instance_api_key": "…" }` ; **`new_contract` est l'id de l'instance** (pas `id`) ; `instance_api_key` est une clé restreinte injectée comme `CONTAINER_API_KEY` qui ne peut que start/stop/destroy cette instance ([create instance](https://docs.vast.ai/api-reference/instances/create-instance.md), [Hello World](https://docs.vast.ai/api-reference/hello-world.md), note « Key Quirks »).
- Limite documentée côté corps de requête : `onstart` ≤ 4048 caractères (note « Key Quirks »).

### 2.2 Lecture d'une instance et états rapportés

- `GET https://console.vast.ai/api/v0/instances/{id}` → `{ "instances": { … } }` ; schéma `Instance` ([show instance](https://docs.vast.ai/api-reference/instances/show-instance.md)) avec notamment :
  - `actual_status` (string|null) : « Current status of the instance container » ;
  - `intended_status`, `cur_state`, `next_state` : statut visé / état courant / état programmé du contrat machine ;
  - `status_msg`, `ssh_host`, `ssh_port`, `ssh_idx`, `public_ipaddr`, `static_ip`, `image_uuid`, `start_date`/`end_date` (epoch s), `duration`, `uptime_mins`, `dph_total`, `storage_total_cost`, `direct_port_count`, `ports`, `num_gpus`, `gpu_ram`, `disk_space`, `geolocation`, … ([show instance](https://docs.vast.ai/api-reference/instances/show-instance.md), [show instances](https://docs.vast.ai/api-reference/instances/show-instances.md)).
- **Valeurs d'`actual_status` documentées** dans le Hello World :
  - `null` → instance en cours de provisionnement ; `"loading"` → téléchargement de l'image Docker ; `"running"` → prête ([Hello World](https://docs.vast.ai/api-reference/hello-world.md)).
  - Statuts « sans issue » à gérer dans la boucle de polling : `"exited"` (conteneur crashé), `"unknown"` (pas de heartbeat), `"offline"` (hôte déconnecté) — ils n'atteindront jamais `running` ; il faut destroy et retenter ([Hello World](https://docs.vast.ai/api-reference/hello-world.md)).
- Le vocabulaire console (guide officiel) ajoute : Creating (création en cours), Loading (téléchargement image), Connecting (Docker tourne, connexion non vérifiée), Scheduling (tentative de restart, en attente GPU), Inactive (stoppée, données préservées), Offline ([Managing Instances](https://docs.vast.ai/guides/instances/manage-instances.md)).
- **« destroyed » n'est pas un état listé** : après destruction l'instance disparaît (« destroyed instances cannot be viewed ») ([Managing Instances](https://docs.vast.ai/guides/instances/manage-instances.md), [destroy instance](https://docs.vast.ai/api-reference/instances/destroy-instance.md)).
- L'adaptateur lit `actual_status`, `intended_status`, `next_state`, `status_msg`, `ssh_host`, `ssh_port`, `image_uuid` et dérive `StartRequested()` de `intended_status`/`next_state == running` (`internal/vast/client.go`) — conforme au schéma.

### 2.3 Délais et coûts pendant l'attente

- Boot typique 1–5 min selon la taille de l'image ; poller toutes les ~10 s ([Hello World](https://docs.vast.ai/api-reference/hello-world.md)).
- « Loading » peut durer ~30 s avec image en cache, ou des heures sur hôte lent ; **non facturé pendant Loading** ([Managing Instances](https://docs.vast.ai/guides/instances/manage-instances.md), [FAQ Billing](https://docs.vast.ai/guides/reference/faq/billing.md), [Billing](https://docs.vast.ai/guides/reference/billing.md)).
- Restart d'une instance stoppée : passe par `SCHEDULING` ; si bloqué > 30 s, le GPU est probablement reloué par un autre utilisateur ; un second stop annule le scheduling ([Managing Instances](https://docs.vast.ai/guides/instances/manage-instances.md)). **Hors docs** : l'API peut répondre à un start par `{"success":false,"error":"resources_unavailable","msg":"…state change queued…"}` (start mis en file) — comportement observé dans les tests de l'adaptateur (`internal/vast/start_test.go`), pas décrit dans l'API reference.

### 2.4 Endpoints de contrôle

- **Stop / Start** : `PUT https://console.vast.ai/api/v0/instances/{id}` avec corps `{"state": "stopped"|"running"}` (l'opération est déterminée par le corps) → `{"success": true}` ([manage instance](https://docs.vast.ai/api-reference/instances/manage-instance.md), [Hello World](https://docs.vast.ai/api-reference/hello-world.md)). L'adaptateur envoie `{"state": ...}` sur `/api/v0/instances/{id}/` (slash final) et exige un ack `success:true` confirmé (`internal/vast/stop.go`).
- **Destroy** : `DELETE https://console.vast.ai/api/v0/instances/{id}` → `{"success": true}` ; irréversible, supprime toutes les données ([destroy instance](https://docs.vast.ai/api-reference/instances/destroy-instance.md), [Hello World](https://docs.vast.ai/api-reference/hello-world.md)). L'adaptateur requiert HTTP 200 + `success:true` (`internal/vast/destroy.go`).
- Stop ≠ destroy : stop = pause du calcul, **les frais de stockage continuent** ; destroy = fin de toute facturation ([Hello World](https://docs.vast.ai/api-reference/hello-world.md), [Billing](https://docs.vast.ai/guides/reference/billing.md), [Managing Instances](https://docs.vast.ai/guides/instances/manage-instances.md)).
- Règles de facturation : GPU facturé à la seconde en état actif/connecté ; stockage à la seconde tant que l'instance existe et est en ligne, **quel que soit l'état** (actif, inactif, loading…), sauf offline ; bande passante au byte dans tout état ; rien facturé si offline ([Billing](https://docs.vast.ai/guides/reference/billing.md), [FAQ Billing](https://docs.vast.ai/guides/reference/faq/billing.md)). Expiration : instances expirées supprimées ~48 h après expiration, non redémarrables ([Managing Instances](https://docs.vast.ai/guides/instances/manage-instances.md), [Instance Types](https://docs.vast.ai/guides/instances/choosing/instance-types.md)).

### 2.5 Coût cumulé rapporté

- Le schéma `Instance` ne documente **pas de champ « total dépensé / coût cumulé »** : les champs de coût portés par l'instance sont des **taux** (`dph_total`, `dph_base`, `min_bid`, `storage_cost`, `storage_total_cost`, sous-objets `search`/`instance` de prix) plus `start_date`, `end_date`, `duration`, `uptime_mins` permettant un calcul local ([show instance](https://docs.vast.ai/api-reference/instances/show-instance.md), [show instances](https://docs.vast.ai/api-reference/instances/show-instances.md)).
- La **dépense réelle** se lit sur `GET https://console.vast.ai/api/v0/charges` (filtre requis `select_filters={"day":{"gte":…,"lte":…}}` en secondes unix, `type` optionnel `in: [instance, volume, serverless]`, format `table`|`tree`, `limit` ≤ 500, pagination par `after_token`) : un objet par contrat avec `source` (`instance-<id>`), `description` (« Instance 12345678 Charges - 4 days »), `amount` (total arrondi à 3 décimales) et `items[]` ventilés par type `gpu` | `disk` | `bwd` | `bwu` (ex. « 96.000 hours at $0.389/hour ») ([show charges](https://docs.vast.ai/api-reference/billing/show-charges.md)).
- Pour les paiements (top-ups Stripe, transferts, payouts) : `show invoices`, distinct de `show charges` ([show charges](https://docs.vast.ai/api-reference/billing/show-charges.md)).

---

## 3. SSH / identités

### 3.1 Ce que l'API rapporte

- L'instance rapporte **host et port SSH** : `ssh_host` (« Host (or IP) used for SSH connection », ex. `ssh2281.vast.ai`), `ssh_port` (entier, ex. `10882`), `ssh_idx`, ainsi que `public_ipaddr`, `static_ip`, `direct_port_count`/`direct_port_start`/`direct_port_end`, `ports` (mapping Docker, présent seulement sur instance running) ([show instance](https://docs.vast.ai/api-reference/instances/show-instance.md), [show instances](https://docs.vast.ai/api-reference/instances/show-instances.md)). `ssh_host`/`ssh_port` sont déjà présents pendant le chargement dans l'exemple Hello World ([Hello World](https://docs.vast.ai/api-reference/hello-world.md)).
- **Pas de champ utilisateur SSH** dans le schéma : l'utilisateur documenté est **`root`** (`ssh root@SSH_HOST -p SSH_PORT` ; `ssh -p 20544 root@142.214.185.187 -L 8080:localhost:8080`) ([SSH guide](https://docs.vast.ai/guides/instances/connect/ssh.md)). Un champ `user` n'existe que côté **création** (option Docker `user`, « User to use with docker create », à utiliser avec précaution) ([create instance](https://docs.vast.ai/api-reference/instances/create-instance.md)).
- **Pas de fingerprint hôte exposé par l'API** : le schéma des endpoints instance ne contient aucun champ de type host key/fingerprint ([show instance](https://docs.vast.ai/api-reference/instances/show-instance.md), [show instances](https://docs.vast.ai/api-reference/instances/show-instances.md)). Les docs ne montrent un fingerprint qu'au moment de la connexion SSH réelle, via l'invite de l'client (`ED25519 key fingerprint is SHA256:WTUphznp…`) ([SSH guide](https://docs.vast.ai/guides/instances/connect/ssh.md)). Conséquence pour un tunnel strict : il n'y a **pas de valeur API à épingler** ; `known_hosts` doit être appris d'une connexion vérifiée (TOFU contrôlé ou canal de confiance hors bande). À noter : deux domaines de confiance distincts possibles — hôte **proxy** Vast (`ssh<N>.vast.ai`, ex. `ssh2281.vast.ai`) vs **IP directe** de la machine ([SSH guide](https://docs.vast.ai/guides/instances/connect/ssh.md), [show instance](https://docs.vast.ai/api-reference/instances/show-instance.md)).
- Authentification : **clés uniquement**, « Password authentication is disabled » ([SSH guide](https://docs.vast.ai/guides/instances/connect/ssh.md), [FAQ Jupyter & SSH](https://docs.vast.ai/guides/reference/faq/jupyter-ssh.md)).

### 3.2 Clés publiques au niveau compte

- Lister : `GET /api/v0/ssh` → tableau d'objets (`id`, `user_id`, `key`, `created_at`, `deleted_at`) ; **404 quand aucune clé** ([show ssh keys](https://docs.vast.ai/api-reference/accounts/show-ssh-keys.md)). L'adaptateur traite le 404 comme « aucune clé » (`internal/vast/client.go`) — conforme.
- Ajouter : `POST /api/v0/ssh`, corps requis `{"ssh_key": "<clé publique .pub>"}` → `{success: true, key: {id, user_id, public_key, …}}` ; la page annonce « The key will be automatically added to all your current instances » ([create ssh-key](https://docs.vast.ai/api-reference/accounts/create-ssh-key.md)).
- Supprimer : `DELETE /api/v0/ssh/{id}` (id numérique de la clé) → `{success: true}` ; la clé est **soft-deletée** (`deleted_at`) ([delete ssh key](https://docs.vast.ai/api-reference/accounts/delete-ssh-key.md), [show ssh keys](https://docs.vast.ai/api-reference/accounts/show-ssh-keys.md)).
- Mise à jour : endpoint `update ssh key` documenté ([llms.txt](https://docs.vast.ai/llms.txt), `api-reference/accounts/update-ssh-key`).
- **Incohérence documentaire à noter** : la page API create ssh-key dit « automatically added to all your current instances », mais le guide SSH (Warning + « SSH Key Changes ») dit le contraire : une clé ajoutée au compte ne s'applique **qu'aux nouvelles instances** ; pour les instances existantes il faut l'interface spécifique à l'instance ; pour les VM, changer de clé impose de recréer la VM ([SSH guide](https://docs.vast.ai/guides/instances/connect/ssh.md)). La note « Key Quirks » des pages OpenAPI tranche : « SSH keys must be registered BEFORE creating an instance (VM: no recovery; Docker: can add post-create) ».

### 3.3 Clés publiques au niveau instance

- Lister les clés d'une instance : `GET /api/v0/instances/{id}/ssh` → `{success, ssh_keys}` ([show ssh-keys (instance)](https://docs.vast.ai/api-reference/instances/show-ssh-keys.md)).
- Attacher : `POST /api/v0/instances/{id}/ssh`, corps `{"ssh_key": "<clé>"}` → `{success: true}` ([attach ssh-key](https://docs.vast.ai/api-reference/instances/attach-ssh-key.md)).
- Détacher : `DELETE /api/v0/instances/{id}/ssh/{ssh_key_id}` (id numérique obtenu via show ssh-keys) → `{success: true}` ([detach ssh-key](https://docs.vast.ai/api-reference/instances/detach-ssh-key.md)).

### 3.4 Permissions de clés API et limites

- Permissions par catégorie : `user_read` → voir clés SSH du compte et `show user` ; `user_write` → create/update/delete clé SSH (compte) ; `instance_read` → logs, clés SSH d'une instance ; `instance_write` → attach/detach clé, create/manage/destroy instance ([Permissions](https://docs.vast.ai/api-reference/permissions.md)). L'adaptateur diagnostique les 403 en citant `user_read`/`user_write` — conforme ([Permissions](https://docs.vast.ai/api-reference/permissions.md), `internal/vast/client.go`).
- **Aucune limite numérique documentée** du nombre de clés SSH (ou de clés API) par compte dans l'API reference ; le seul plafond mentionné concerne les variables d'environnement (« There is a limit on the total number of environment variables per user ») ([llms.txt](https://docs.vast.ai/llms.txt) — description de create env-var).

### 3.5 Logs (dépendance du flux de préparation)

- `PUT /api/v0/instances/request_logs/{id}` (corps optionnel : `tail`, `filter`, `daemon_logs` = `"true"` pour les logs système daemon) → `{success: true, result_url: <URL S3>}` ; les logs sont téléversés sur S3 et récupérés via l'URL ([show logs](https://docs.vast.ai/api-reference/instances/show-logs.md)). L'adaptateur appelle exactement cet endpoint avec `{"daemon_logs":"true","tail":"40"}` puis restreint l'URL de résultat aux buckets `vast.ai/instance_logs/` / `public.vast.ai/instance_logs/` sur `s3.amazonaws.com` (`internal/vast/logs.go`) — conforme.

---

## 4. Facturation / garde-fous

### 4.1 Champs de coût côté API

- Solde : `GET /api/v0/users/current` (show user) → champs documentés `id`, `key_id`, `email`, **`balance`** (« The current balance of the user »), `ssh_key`, `sid` ([show user](https://docs.vast.ai/api-reference/accounts/show-user.md)). **Hors docs** : l'adaptateur lit aussi `credit` (champ non documenté, présent dans les réponses réelles) et utilise `balance` en secours quand le crédit est négatif/invalide (`internal/vast/account.go`).
- Taux par instance : `dph_total`, `dph_base`, `min_bid`, `is_bid`, `storage_cost`, `storage_total_cost`, `vram_costperhour`, sous-objets `search`/`instance` de prix, `credit_discount`/`credit_balance` (dépréciés, ex. `null`) ([show instance](https://docs.vast.ai/api-reference/instances/show-instance.md), [show instances](https://docs.vast.ai/api-reference/instances/show-instances.md)).
- Historique facturé : `GET /api/v0/charges` (montants par contrat, ventilation gpu/disk/bwd/bwu) et `show invoices` (paiements) ([show charges](https://docs.vast.ai/api-reference/billing/show-charges.md)).

### 4.2 Caps et garde-fous

- **Aucun endpoint de « cap de dépense » / budget / plafond monétaire n'est documenté dans l'API reference** (aucune mention de cap, budget ou spend limit dans les pages API ; le schéma `show user` ne contient que le solde) ([show user](https://docs.vast.ai/api-reference/accounts/show-user.md), [Permissions](https://docs.vast.ai/api-reference/permissions.md), [llms.txt](https://docs.vast.ai/llms.txt)).
- Les garde-fous existants sont :
  - **Pré-paiement obligatoire** : Vast exige des crédits avant location ; pas de facturation post-payée ([Billing](https://docs.vast.ai/guides/reference/billing.md), [FAQ Billing](https://docs.vast.ai/guides/reference/faq/billing.md)).
  - **Stop automatique à solde ≤ 0** : quand le crédit atteint 0, les instances sont stoppées automatiquement ; le stockage continue d'être facturé ; avec carte enregistrée, la carte est chargée automatiquement pour couvrir le négatif ; sans carte, les instances et données peuvent être **détruites** ; une marge (buffer) calculée sur la dépense journalière moyenne permet un court passage en négatif ([Billing](https://docs.vast.ai/guides/reference/billing.md), [FAQ Billing](https://docs.vast.ai/guides/reference/faq/billing.md)).
  - **Console (non API)** : seuil d'**autobilling** (« set a balance threshold to configure auto billing ») et **notification email de solde bas** (seuil recommandé ~75 % du seuil d'autobilling) ([Billing](https://docs.vast.ai/guides/reference/billing.md), [FAQ Billing](https://docs.vast.ai/guides/reference/faq/billing.md)).
  - **Primitives API utiles aux garde-fous applicatifs** : `dph_total` lisible à la recherche et sur l'instance (affichage du coût avant achat), `cancel_unavail` et `target_state` à la création, `destroy` pour couper toute facturation, solde via `show user`, montants via `show charges` ([create instance](https://docs.vast.ai/api-reference/instances/create-instance.md), [search offers](https://docs.vast.ai/api-reference/search/search-offers.md), [show user](https://docs.vast.ai/api-reference/accounts/show-user.md), [show charges](https://docs.vast.ai/api-reference/billing/show-charges.md)).
- Fréquence de mise à jour des soldes : « about once every few seconds » ([Billing](https://docs.vast.ai/guides/reference/billing.md), [FAQ Billing](https://docs.vast.ai/guides/reference/faq/billing.md)).

---

## 5. Croisement avec l'adaptateur `internal/vast` (`feat/recipe-picker-tui`)

| Élément adaptateur (fichier) | Appel/champ | Source API correspondante | Verdict |
|---|---|---|---|
| `search.go` | `POST /api/v0/bundles` ; filtres `limit`, `type=ondemand`, `verified/rentable/rented eq`, `gpu_ram gte` (Mo), `disk_space gte` (Go), `geolocation in`, `gpu_name/num_gpus eq` (strict), `order` | [search offers](https://docs.vast.ai/api-reference/search/search-offers.md) | Conforme (unités Mo/Go respectées) |
| `client.go` (Create) | `PUT /api/v0/asks/{id}` ; `image`, `disk`, `runtype: ssh_direct`, `label` | [create instance](https://docs.vast.ai/api-reference/instances/create-instance.md) | Conforme ; réponse `new_contract` attendu |
| `client.go` (Get) | `GET /api/v0/instances/{id}` ; lit `actual_status`, `intended_status`, `next_state`, `status_msg`, `ssh_host`, `ssh_port` | [show instance](https://docs.vast.ai/api-reference/instances/show-instance.md) | Conforme |
| `destroy.go` (Exists) | `GET /api/v0/instances/{id}/?owner=me` | paramètre `owner` **hors docs** (l'exemple Hello World liste sans ce paramètre) | Non documenté ; vérifier lors du portage |
| `stop.go` | `PUT /api/v0/instances/{id}/` `{"state":"stopped"|"running"}` ; exige `success:true` | [manage instance](https://docs.vast.ai/api-reference/instances/manage-instance.md) | Conforme ; la gestion « queued » est **hors docs** |
| `destroy.go` | `DELETE /api/v0/instances/{id}` ; exige HTTP 200 + `success:true` | [destroy instance](https://docs.vast.ai/api-reference/instances/destroy-instance.md) | Conforme |
| `logs.go` | `PUT /api/v0/instances/request_logs/{id}/` `{"daemon_logs":"true","tail":"40"}` ; puis GET de `result_url` (S3) | [show logs](https://docs.vast.ai/api-reference/instances/show-logs.md) | Conforme |
| `client.go` (SSH) | `GET /api/v0/ssh/` (404 = aucune clé), `POST /api/v0/ssh/` `{"ssh_key":…}` | [show ssh keys](https://docs.vast.ai/api-reference/accounts/show-ssh-keys.md), [create ssh-key](https://docs.vast.ai/api-reference/accounts/create-ssh-key.md) | Conforme |
| `account.go` | `GET /api/v0/users/current/` ; lit `credit` puis `balance` | [show user](https://docs.vast.ai/api-reference/accounts/show-user.md) (seul `balance` est documenté) | Champ `credit` **hors docs** |
| `regions.go` / `countries.go` | Aucun appel réseau ; catalogues CLDR locaux (région→codes pays) | pas d'endpoint Vast région/pays ; filtre API = codes pays 2 lettres | Conforme |
| — | `activity.go` **n'existe pas** sur la branche (seul `activity_test.go` : décodage `status_msg` + `ssh_host`/`ssh_port` au statut `running`) | [show instance](https://docs.vast.ai/api-reference/instances/show-instance.md) | — |

Points d'attention pour les tickets dépendants (#5/#6) :
1. `gpu_name` : égalité stricte sur le nom canonique ; attention espaces vs soulignés selon le canal (REST/CLI) — voir §1.2.
2. Pas de coût cumulé dans le payload instance : lire `/api/v0/charges` (fenêtre de dates requise) ou accumuler localement depuis `start_date` + `dph_total` (§2.5, §4.1).
3. Pas de fingerprint hôte dans l'API : le tunnel strict doit apprendre la clé d'une connexion vérifiée (§3.1).
4. Une clé SSH compte doit être posée **avant** création pour couvrir le cas VM ; en Docker, rattachement possible après coup (§3.2/3.3).
5. Pas de cap de dépense côté API : garde-fou applicatif à construire (solde `balance`, taux `dph_total`, charges) ; seuls stop-à-zéro et autobilling console existent côté Vast (§4.2).

## Sources

Consultées le 2026-09-07 (toutes les pages servent aussi un rendu markdown via l'ajout de `.md`) :
- Index complet de la doc : https://docs.vast.ai/llms.txt
- API Hello World (cycle de vie complet, table des `actual_status`) : https://docs.vast.ai/api-reference/hello-world
- API reference — search offers : https://docs.vast.ai/api-reference/search/search-offers
- API reference — create instance : https://docs.vast.ai/api-reference/instances/create-instance
- API reference — show instance : https://docs.vast.ai/api-reference/instances/show-instance
- API reference — show instances : https://docs.vast.ai/api-reference/instances/show-instances
- API reference — manage instance (stop/start) : https://docs.vast.ai/api-reference/instances/manage-instance
- API reference — destroy instance : https://docs.vast.ai/api-reference/instances/destroy-instance
- API reference — show logs : https://docs.vast.ai/api-reference/instances/show-logs
- API reference — show user : https://docs.vast.ai/api-reference/accounts/show-user
- API reference — show ssh keys (compte) : https://docs.vast.ai/api-reference/accounts/show-ssh-keys
- API reference — create ssh-key : https://docs.vast.ai/api-reference/accounts/create-ssh-key
- API reference — delete ssh key : https://docs.vast.ai/api-reference/accounts/delete-ssh-key
- API reference — show ssh-keys (instance) : https://docs.vast.ai/api-reference/instances/show-ssh-keys
- API reference — attach ssh-key : https://docs.vast.ai/api-reference/instances/attach-ssh-key
- API reference — detach ssh-key : https://docs.vast.ai/api-reference/instances/detach-ssh-key
- API reference — permissions : https://docs.vast.ai/api-reference/permissions
- API reference — show charges : https://docs.vast.ai/api-reference/billing/show-charges
- CLI — search offers : https://docs.vast.ai/cli/reference/search-offers
- Guide — SSH : https://docs.vast.ai/guides/instances/connect/ssh
- Guide — Managing Instances (états console, restart) : https://docs.vast.ai/guides/instances/manage-instances
- Guide — Instance Types (on-demand/reserved/interruptible) : https://docs.vast.ai/guides/instances/choosing/instance-types
- Guide — Billing : https://docs.vast.ai/guides/reference/billing
- FAQ — Billing : https://docs.vast.ai/guides/reference/faq/billing
- FAQ — Jupyter & SSH : https://docs.vast.ai/guides/reference/faq/jupyter-ssh
- FAQ — Security : https://docs.vast.ai/guides/reference/faq/security
- Code croisé (branch `feat/recipe-picker-tui`) : `git show origin/feat/recipe-picker-tui:internal/vast/{client,search,stop,destroy,logs,regions,countries,account}.go` et `*_test.go`
