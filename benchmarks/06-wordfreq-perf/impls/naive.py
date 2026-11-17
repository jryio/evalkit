# Counts each distinct word by scanning the whole list again.
import json, sys
words = open(sys.argv[1]).read().split()
counts = [(words.count(w), w) for w in set(words)]
counts.sort(key=lambda c: (-c[0], c[1]))
print(json.dumps([[w, n] for n, w in counts[:10]]))
