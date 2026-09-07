# Sovereign Kit

CLI Go de provisionnement de modèles privés : depuis une recette, louer une instance GPU sur Vast, la préparer, ouvrir un tunnel SSH strict et exposer une route OpenAI-compatible locale pour des harness de code.

## Language

**recette** (*recipe*):
Contrat immuable : moteur d'inférence (llama-cpp, sglang, vllm), modèle épinglé (dépôt, révision, digest/fichier), réglages de service, requirements matériels. Vit dans un fichier TOML versionné, embarqué dans le binaire.
_Avoid_: config, profile, template, offre

**instance**:
Machine GPU louée sur Vast via l'API, identifiée par son id Vast. Objet du cycle de vie : location → prêt → service → arrêt → destruction.
_Avoid_: machine, hôte, serveur

**déploiement** (*deployment*):
Enregistrement local qui relie recette + instance + clés SSH + port alloué + dépense cumulée. Unité que le CLI liste et gère. Un actif à la fois, mais stock par enregistrements (pas de singleton).
_Avoid_: config, profil, session

**route**:
Endpoint OpenAI-compatible local d'un déploiement, `127.0.0.1:<port alloué>`, atteint via tunnel SSH strict. Ce que les harness consomment.
_Avoid_: endpoint distant, URL publique, provider

**tunnel**:
Forward SSH local (loopback local → loopback distant) avec vérification stricte du host key.
_Avoid_: proxy, gateway, passerelle

**garde-fous**:
Règles de sécurité opérationnelle du provisionnement : jamais de port public, images et modèles épinglés, confirmation avant achat, cap de dépense, interruptible interdit par défaut, destroy explicite.
_Avoid_: policies, règles métier
