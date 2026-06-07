FROM node:22-alpine AS builder

WORKDIR /build/web
COPY web/package.json ./
COPY web/default/package.json ./default/package.json
RUN mkdir -p classic && echo '{"name":"classic-placeholder","private":true}' > classic/package.json
# Inline bun catalog: references for npm compatibility
RUN node -e "
  const root = JSON.parse(require('fs').readFileSync('package.json','utf8'));
  const catalog = root.catalog || {};
  const def = JSON.parse(require('fs').readFileSync('default/package.json','utf8'));
  for (const [k,v] of Object.entries(def.dependencies||{})) {
    if (v==='catalog:') def.dependencies[k]=catalog[k]||'*';
  }
  for (const [k,v] of Object.entries(def.devDependencies||{})) {
    if (v==='catalog:') def.devDependencies[k]=catalog[k]||'*';
  }
  require('fs').writeFileSync('default/package.json', JSON.stringify(def));
"
RUN npm install
COPY ./web/default ./default
COPY ./VERSION /build/VERSION
RUN cd default && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) npm run build

FROM node:22-alpine AS builder-classic

WORKDIR /build/web
COPY web/package.json ./
COPY web/classic/package.json ./classic/package.json
RUN mkdir -p default && echo '{"name":"default-placeholder","private":true}' > default/package.json
# Inline bun catalog: references for npm compatibility
RUN node -e "
  const root = JSON.parse(require('fs').readFileSync('package.json','utf8'));
  const catalog = root.catalog || {};
  const cls = JSON.parse(require('fs').readFileSync('classic/package.json','utf8'));
  for (const [k,v] of Object.entries(cls.dependencies||{})) {
    if (v==='catalog:') cls.dependencies[k]=catalog[k]||'*';
  }
  for (const [k,v] of Object.entries(cls.devDependencies||{})) {
    if (v==='catalog:') cls.devDependencies[k]=catalog[k]||'*';
  }
  require('fs').writeFileSync('classic/package.json', JSON.stringify(cls));
"
RUN npm install
COPY ./web/classic ./classic
COPY ./VERSION /build/VERSION
RUN cd classic && VITE_REACT_APP_VERSION=$(cat /build/VERSION) npm run build


FROM golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=builder /build/web/default/dist ./web/default/dist
COPY --from=builder-classic /build/web/classic/dist ./web/classic/dist
RUN go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

RUN sed -i 's|deb.debian.org|ftp.hk.debian.org|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null; \
    apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY --from=builder2 /build/new-api /
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
