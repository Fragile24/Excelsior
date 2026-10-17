# Bundled DeepSeek tokenizers

These assets come from `deepseek-ai/deepseek-recipe` through the pinned pure Go
port `github.com/dandandujie/dsr-go`, revision
`3fd324c60f95aaf55c7b33c80eebfc243036f15b` (2026-09-10).
The prompt conversion, rendering, and BPE implementation use that same revision.

- `deepseek-v4.json.gz` is the unmodified V4 tokenizer JSON, gzip compressed.
  Uncompressed SHA-256: `97d2f31b020d18b5aee5c9b3d5b4efb10ea210f3fe3f7dffe3f1cd90542d6b19`.
  Source: [V4 tokenizer](https://github.com/dandandujie/dsr-go/blob/3fd324c60f95aaf55c7b33c80eebfc243036f15b/static/tokenizers/v4/tokenizer.json).
- `deepseek-v41-tokens.json` contains the eight `added_tokens` replacements for
  V4.1, indexed into the V4 array. Every other JSON field is identical between
  the upstream files. This avoids embedding a second copy of the vocabulary.
  Upstream V4.1 JSON SHA-256: `81f64d1248a68ce3663e07ab3ee48b851e5df0e32d27cb98e4c9a268151e8d99`.
  Source: [V4.1 tokenizer](https://github.com/dandandujie/dsr-go/blob/3fd324c60f95aaf55c7b33c80eebfc243036f15b/static/tokenizers/v41/tokenizer.json).

The upstream MIT license is preserved in `LICENSE`. These are text-only prompt
counts. The upstream image-token reassignment does not change text input counts.

To refresh, pin a new port revision, gzip its V4 tokenizer JSON, and compare both
JSON documents. Require all fields except `added_tokens` to match, then save
each changed array entry as `{"index":N,"token":{...}}` in the V4.1 delta.
Update these hashes and provenance with the dependency. No files are downloaded
at runtime.
