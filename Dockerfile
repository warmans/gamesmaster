FROM debian:stable-slim

RUN apt update && apt install -y ca-certificates libfreetype-dev wbritish

RUN mkdir -p /opt/gamesmaster/var/crossword/game && mkdir -p /opt/gamesmaster/var/crossword/wordlist && mkdir -p /opt/gamesmaster/etc && chown -R nobody /opt/gamesmaster

RUN addgroup nobody

ARG USER=nobody
USER nobody

WORKDIR /opt/gamesmaster

COPY --chown=nobody bin/gamesmaster .
COPY --chown=nobody etc/sowpods.txt etc/.

RUN chmod +x gamesmaster

CMD ["/opt/gamesmaster/gamesmaster", "bot"]
