# Benchmark mono-agent 262K — historique, non recette de production

> **Avis sécurité :** cette capture historique utilise une image de développement et ne fixe pas la révision Hugging Face. Ne pas la reproduire telle quelle pour une charge client. La recette de production révisée est dans [`docs/server-launch.md`](../docs/server-launch.md), avec révision du modèle épinglée, contrôles de tunnel et exigences de revue supplémentaires.

Date : 2026-09-01. Test synthétique uniquement ; aucun code ou contexte client n'a été transmis.

## Objectif

Valider qu'un seul serveur Qwen3.8-27B peut accepter une requête proche de sa fenêtre native de 262 144 tokens, puis générer une sortie longue.

## Infrastructure

- GPU : NVIDIA RTX PRO 6000 Blackwell Server Edition, 97 887 MiB VRAM.
- Runtime : image SGLang `lmsysorg/sglang:dev-qwen38-27b-dflash2`.
- Modèle : `RadixArk/Qwen3.8-27B-NVFP4`, dérivé quantifié NVFP4 de Qwen3.8-27B.
- Endpoint : lié à `127.0.0.1:30000` sur le pod.
- Concurrence : 1.

## Commande serveur

```bash
sglang serve \
  --trust-remote-code \
  --model-path RadixArk/Qwen3.8-27B-NVFP4 \
  --context-length 262144 \
  --kv-cache-dtype fp8_e4m3 \
  --mem-fraction-static 0.85 \
  --attention-backend flashinfer \
  --chunked-prefill-size 2048 \
  --max-running-requests 1 \
  --cuda-graph-max-bs 1 \
  --reasoning-parser qwen3 \
  --tool-call-parser qwen3_coder \
  --host 127.0.0.1 --port 30000
```

## Charge générée

- Prompt tokenisé localement avec le tokenizer du checkpoint : **246 000 tokens exacts**.
- `max_tokens` : **8 192**.
- Réponse API : HTTP 200, `finish_reason: length`.
- Usage retourné par le serveur : `prompt_tokens: 246000`, `completion_tokens: 8192`, `total_tokens: 254192`.

## Mesures observées

- Après chargement des poids : 20.14 Go utilisés, 74.13 Go disponibles.
- Pool Mamba : 0.54 Go d'état convolution + 27.70 Go d'état SSM.
- Pool KV FP8 réservé : 1 035 364 tokens, K 15.80 Go + V 15.80 Go.
- Après graph capture : ~12.33 Go disponibles.
- Pendant la requête proche de 254K tokens : 85 749 / 97 887 MiB utilisés, GPU à 100 %.
- Decode à contexte ~246K–254K : ~48.2 tok/s, suivant les logs SGLang.

## Conclusion

Le serveur a réellement accepté puis généré à partir de 246K tokens d'entrée et 8 192 tokens de sortie, sans OOM ni troncature. Cette RTX PRO 6000 S 96 Go est donc une base viable pour **un** agent à contexte total 262K avec ce runtime/modèle. Cette charge synthétique valide la capacité et le débit, pas la qualité de raisonnement sur un vrai dépôt.

## Limites / suivi

- Le runtime a signalé que le KV FP8 n'avait pas de facteurs de scaling fournis et utilisait 1.0 ; il faut comparer la qualité aux KV BF16 ou à une quantification calibrée sur une charge de code réelle.
- Le test ne valide ni plusieurs agents, ni les 262K tokens utiles de code hétérogène, ni la qualité long-context.
- L'instance de test a été détruite après le résultat.
