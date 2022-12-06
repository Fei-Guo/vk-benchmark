# Start from latest golang base image
FROM golang:latest as builder

# Set the current directory inside the container
WORKDIR /app

COPY go.mod .
COPY go.sum .
COPY Makefile .

# Copy sources inside the docker
#COPY pkg/ pkg/
COPY cmd/ cmd/

# install the dependencies
RUN go mod tidy

# Build the binaries from the source
RUN make

###### Start a new stage from scratch #######
FROM ubuntu:latest

WORKDIR /

COPY --from=builder /app/_output/bin/vk-benchmark .

# Expose port 8080 to the outside container
EXPOSE 8082

