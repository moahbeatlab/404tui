# A plain Linux (Ubuntu) to run the program in a terminal (tmux) against a real project, as a user would.
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends tmux ca-certificates libasound2t64 python3-minimal && rm -rf /var/lib/apt/lists/*
ENV TERM=tmux-256color COLORTERM=truecolor LANG=C.UTF-8
COPY e2e-lib.sh e2e.sh e2e-tools.sh e2e-new.sh hl.py all.sh /
ENTRYPOINT ["/bin/bash", "/all.sh"]
