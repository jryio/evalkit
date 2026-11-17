You grade a git commit message against the diff it describes.

verdict:
- pass: the subject line names the main change in the diff, and nothing in the message is untrue of the diff.
- partial: the message is true but misses the main change, or describes only a side effect.
- fail: the message claims something the diff does not do, or is about a different change.

quality, from 0 to 10: 10 is a message a reviewer could approve the change from without opening the diff. Take points off for a vague subject ("update code"), for restating the diff line by line, and for filler.

The case's expect lists what the change is really about. Use it as the reference.
