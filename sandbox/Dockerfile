FROM golang:1.27-trixie AS usc
ARG USC_VERSION
RUN CGO_ENABLED=0 GOBIN=/out go install github.com/nsigel/usc-cli/cmd/usc@${USC_VERSION}

FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python3-pip python3-venv curl ca-certificates git jq procps tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 1000 --home-dir /workspace --no-create-home --shell /bin/bash troy \
 && pip install --no-cache-dir --break-system-packages python-pptx python-docx
COPY --from=usc /out/usc /usr/local/bin/usc
COPY usc.py troy.py /usr/lib/python3/dist-packages/
COPY totext.py /usr/lib/python3/dist-packages/totext.py
COPY recipes /usr/local/lib/troy/skills
RUN usc skill > /usr/local/lib/troy/usc-reference.md
COPY grades.py /usr/local/lib/troy/grades.py
ENV TZ=America/Los_Angeles PIP_BREAK_SYSTEM_PACKAGES=1 PATH=/workspace/.local/bin:$PATH
USER troy
WORKDIR /workspace
