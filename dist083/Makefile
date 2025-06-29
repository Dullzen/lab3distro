# Makefile para dist083 (partida3)

all: docker-servidor3

# Build partida3
build-partida3:
    docker build -t partida3 ../partidas

# Run partida3
docker-servidor3: build-partida3
    -docker rm -f partida3 || true
    # Run partida3 with environment variables
    docker run --rm -d -p 50054:50054 --name partida3 \
        -e PARTIDA_ID=Partida-3 \
        -e PARTIDA_PORT=50054 \
        -e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
        partida3

# Clean up
clean:
    -docker rm -f partida3 || true
    docker rmi -f partida3