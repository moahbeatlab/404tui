# An ArkOS-like arm64 userspace to run the launcher and sp404tui-arm64 in, on any machine.
# ArkOS 2.0 is Ubuntu 19.10 (eoan): same glibc and package versions as the R36S.
# If eoan cannot be pulled any more, build with:  docker compose build --build-arg BASE=ubuntu:20.04
ARG BASE=ubuntu:eoan
FROM --platform=linux/arm64 ${BASE}

# eoan is end of life: its packages live on old-releases. Harmless on a newer BASE (no match, no change).
RUN sed -i -E 's#https?://(ports|archive|security)\.ubuntu\.com/ubuntu(-ports)?#http://old-releases.ubuntu.com/ubuntu#g' /etc/apt/sources.list 2>/dev/null; \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      sudo util-linux procps file libasound2 alsa-utils kmod ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# the user and the groups of the device (ark, uid 1002, passwordless sudo), and the folders the launcher expects
RUN groupadd -g 1002 ark && useradd -m -u 1002 -g 1002 -G sudo,audio,video -s /bin/bash ark \
    && echo 'ark ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/ark \
    && mkdir -p /roms /mnt/usbdrive && chown ark:ark /roms /mnt/usbdrive

USER ark
WORKDIR /roms
# bash, not a service: the launcher is what the device runs from the Ports menu
CMD ["/bin/bash", "/roms/ports/404tui.sh"]
