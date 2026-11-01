# mail-sink

Local SMTP/HTTP sink that validates a token on every send, so withdrawing the token in a drill
produces a deterministic, repeatable failure. Stores received mail for assertions.
