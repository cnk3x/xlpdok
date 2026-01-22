FROM --platform=${TARGETARCH} debian:stable-slim
ARG TARGETARCH

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && mkdir -p /rootfs/etc/ssl/certs /rootfs/lib \
  && cp -Lr /usr/share/zoneinfo/Asia/Shanghai /rootfs/etc/localtime \
  && echo "Asia/Shanghai" >/rootfs/etc/timezone \
  && cp -Lr --parents /etc/ssl/certs/ca-certificates.crt /rootfs/ \
  && find /usr/lib \( -name libdl.so.2 -o -name libgcc_s.so.1 -o -name libstdc++.so.6 \) -exec cp -Lr {} /rootfs/lib/ \;

COPY artifacts/xlpdok-linux-${TARGETARCH} /rootfs/xlpdok

FROM --platform=${TARGETARCH} busybox:1.37
ARG TARGETARCH
COPY --from=0 /rootfs /

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
  DownloadPATH=/downloads \
  GIN_MODE=release

CMD [ "/xlpdok" ]
