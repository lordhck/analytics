FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
        curl unzip ca-certificates sudo \
    && rm -rf /var/lib/apt/lists/*

ARG USER=dev
ARG UID=1000
RUN useradd -m -u ${UID} -s /bin/bash ${USER} \
    && echo "${USER} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/${USER}

USER ${USER}
WORKDIR /app

RUN curl -fsSL https://bun.sh/install | bash
ENV BUN_INSTALL=/home/dev/.bun
ENV PATH=$BUN_INSTALL/bin:$PATH

EXPOSE 5173

CMD ["sleep", "infinity"]
