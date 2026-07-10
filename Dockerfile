# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.26 AS build
WORKDIR /src

# Cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Build a static binary. time/tzdata is embedded in the source so no OS tzdata
# is needed at runtime.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags "-s -w" \
    -o /out/hyundai-bluelink-mqtt ./cmd

# --- runtime stage ---
FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY --from=build /out/hyundai-bluelink-mqtt /hyundai-bluelink-mqtt

# Prod defaults: persist tokens to a Kubernetes Secret. The health/probe port.
ENV TOKEN_STORE=kube
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/hyundai-bluelink-mqtt"]
CMD ["serve"]
