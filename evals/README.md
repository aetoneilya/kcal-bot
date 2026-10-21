# Weighed meals

Ground truth for `kcal-bot -eval evals`: one directory per meal, with one to
three photos and a `truth.json` that says what the kitchen scale said.

```
evals/2026-10-22-buckwheat-chicken/
  photo.jpg
  truth.json   {"kcal": 612, "note": "", "items": [{"name": "гречка", "grams": 180}, {"name": "курица", "grams": 150}]}
```

- Photograph the plate as you normally would, then weigh each part.
- `kcal` is the total worked out from the weights and the labels (or a
  nutrition table) — the number the eval scores against.
- `note` is what you would type in the app, if anything.
- 20–30 everyday meals are enough to compare prompts and models; a run costs
  about a cent per meal.

Run it on the server, where the model key lives:

```
sudo -u kcal bash -c 'set -a; . /opt/kcal-bot/.env; set +a; cd /tmp; /opt/kcal-bot/kcal-bot -eval /path/to/evals'
```

Photos of your food stay out of git (`evals/*/` is ignored); only this file is
tracked.
