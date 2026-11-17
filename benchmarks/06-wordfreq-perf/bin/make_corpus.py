#!/usr/bin/env python3
# Writes the same pseudo-random texts into every slot: a fixed seed, so every
# run and every machine times the same input.
import json, os, random
suite = os.environ["EVALKIT_SUITE_DIR"]
rng = random.Random(7)
vocab = ["w%04d" % i for i in range(1500)]
weights = [1 / (i + 1) for i in range(len(vocab))]  # a few common words, a long tail
os.makedirs("corpus", exist_ok=True)
for line in open(os.path.join(suite, "cases.jsonl")):
    case = json.loads(line)
    words = rng.choices(vocab, weights, k=case["words"])
    open(case["file"], "w").write(" ".join(words) + "\n")
