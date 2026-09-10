# nett — minimal, self-contained container.
#
# The image contains ONLY the Go toolchain (build stage) and the compiled nett
# binary plus CA certificates (final stage). It installs NONE of subfinder,
# amass, httpx, dnsx, naabu, nuclei, ffuf, gobuster, katana, or any other recon
# tool: the framework's core functionality is implemented natively in Go.
#
# Build:  docker build -t nett .
# Run:    docker run --rm nett capabilities

# ---- build stage ----
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod ./
# go.sum is optional while there are no third-party deps; copy if present.
COPY . .
# CGO disabled so the binary is static and runs on a distroless/scratch base.
# A pure-Go SQLite driver is used in later milestones to keep this true.
ENV CGO_ENABLED=0
RUN go build -ldflags "-s -w" -o /out/nett ./cmd/nett

# ---- final stage: minimal, no security tools ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nett /usr/local/bin/nett
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/nett"]
CMD ["--help"]
