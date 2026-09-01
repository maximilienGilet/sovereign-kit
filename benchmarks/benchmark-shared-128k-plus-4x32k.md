# Benchmark partagé Qwen — 128K principal + 4×32K sous-agents

Date : 2026-09-01. Charge synthétique, sans code ni contexte client. Instance détruite après le test.

## Infrastructure et serveur

- GPU : RTX PRO 6000 S, 97 887 MiB.
- Prix observé : $1.4333/h.
- Runtime : SGLang `lmsysorg/sglang:dev-qwen38-27b-dflash2`.
- Modèle : `RadixArk/Qwen3.8-27B-NVFP4`.
- Contexte maximum serveur : 262 144.
- KV cache : FP8 E4M3 ; attention : FlashInfer.
- `max-running-requests=5`, `cuda-graph-max-bs=5`.

## Charge simultanée

Les cinq requêtes ont été envoyées simultanément au même endpoint OpenAI-compatible :

| Rôle | Prompt exact | Sortie demandée |
|---|---:|---:|
| Principal | 128 000 tokens | 4 096 tokens |
| Sous-agent 1–4 | 32 000 tokens chacun | 1 024 tokens chacun |

Total d'entrées actives : 256 000 tokens. Total de sortie : 8 192 tokens.

## Résultats

| Rôle | HTTP | TTFT | Durée E2E | Sortie | Débit après premier token |
|---|---:|---:|---:|---:|---:|
| Principal | 200 | 60.165 s | 133.687 s | 4 096 | 55.712 tok/s |
| Sous-agent 1 | 200 | 9.862 s | 33.028 s | 1 024 | 44.203 tok/s |
| Sous-agent 2 | 200 | 13.079 s | 33.028 s | 1 024 | 51.332 tok/s |
| Sous-agent 3 | 200 | 3.406 s | 33.027 s | 1 024 | 34.570 tok/s |
| Sous-agent 4 | 200 | 6.720 s | 33.028 s | 1 024 | 38.924 tok/s |

- Débit agrégé des quatre sous-agents sur leur fenêtre de 33.028 s : **124.02 tok/s**.
- Débit agrégé de la charge complète (8 192 tokens / 133.687 s) : **61.28 tok/s**.
- Mémoire observée pendant la charge : 85 837 / 97 887 MiB ; marge ~11.77 GiB.

## Conclusion

Le même serveur Qwen a accepté et servi le profil sélectionné sans OOM ni file bloquée : un principal à 128K et quatre sous-agents à 32K. Les sous-agents sont revenus en ~33 s, pendant que le principal continuait sa génération longue.

Le principal souffre d'un TTFT de ~60 s car son préfill 128K est en concurrence avec les quatre prefills 32K ; le compromis de production doit donc être une politique de priorité, plutôt qu'envoyer les cinq gros prompts exactement au même instant.

Ce benchmark valide capacité + concurrence avec prompts synthétiques. Il ne valide ni la qualité sur dépôts réels, ni le meilleur ordonnanceur pour le harness.
