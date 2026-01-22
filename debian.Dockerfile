FROM --platform=${TARGETARCH} debian:stable-slim
ARG TARGETARCH

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && rm -f localtime && cp -Lr /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
  && echo "Asia/Shanghai" >/etc/timezone

COPY artifacts/xlpdok-linux-${TARGETARCH} /xlpdok

CMD [ "/xlpdok" ]

LABEL org.opencontainers.image.authors=cnk3x \
  org.opencontainers.image.source=https://github.com/cnk3x/xlpdok \
  org.opencontainers.image.description="迅雷远程下载服务(非官方)" \
  org.opencontainers.image.licenses=MIT
