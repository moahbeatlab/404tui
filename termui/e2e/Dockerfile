# A plain Linux (Ubuntu) to run the lab in a terminal (tmux) against a real project, as a user would.
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends tmux ca-certificates libasound2t64 && rm -rf /var/lib/apt/lists/*
ENV TERM=tmux-256color COLORTERM=truecolor LANG=C.UTF-8
COPY e2e.sh /e2e.sh
ENTRYPOINT ["/bin/bash", "/e2e.sh"]
