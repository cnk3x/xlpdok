FROM --platform=${TARGETARCH} debian:stable-slim
ARG TARGETARCH

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && rm -f localtime && cp -Lr /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
  && echo "Asia/Shanghai" >/etc/timezone

COPY artifacts/xlpdok-linux-${TARGETARCH} /xlpdok

CMD [ "/xlpdok" ]

ENV SYNOPLATFORM=geminilake \
  SYNOPKG_PKGNAME=pan-xunlei-com \
  SYNOPKG_PKGDEST=/var/packages/pan-xunlei-com/target \
  SYNOPKG_DSM_VERSION_MAJOR=7 \
  SYNOPKG_DSM_VERSION_MINOR=2 \
  SYNOPKG_DSM_VERSION_BUILD=64570 \
  DriveListen=unix:///var/packages/pan-xunlei-com/target/var/pan-xunlei-com.sock \
  PLATFORM=群晖 \
  OS_VERSION="geminilake dsm 7.2-64570" \
  ConfigPath=/data \
  HOME=/data/.drive \
  DownloadPATH= \
  GIN_MODE=release

LABEL org.opencontainers.image.authors=cnk3x \
  org.opencontainers.image.source=https://github.com/cnk3x/xlpdok \
  org.opencontainers.image.description="迅雷远程下载服务(非官方)" \
  org.opencontainers.image.licenses=MIT
