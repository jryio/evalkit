# Rate limits

Each API key may make 600 requests per minute. A request over the limit gets status 429 with a `Retry-After` header in seconds.

Batch endpoints count as one request per item in the batch, up to 100 items.
