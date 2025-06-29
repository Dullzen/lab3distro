# Makefile para dist082 (cliente2 y partida2)

all: docker-jugador2-servidor2

# Build cliente2
build-cliente2:
    docker build -t cliente2 ../cliente

# Build partida2
build-partida2:
    docker build -t partida2 ../partidas

# Run both cliente2 and partida2
docker-jugador2-servidor2: build-cliente2 build-partida2
    -docker rm -f cliente2 || true
    -docker rm -f partida2 || true
    # Run partida2 with environment variables
    docker run -d --rm -p 50053:50053 --name partida2 \
        -e PARTIDA_ID=Partida-2 \
        -e PARTIDA_PORT=50053 \
        -e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
        partida2
    # Run cliente2 with interactive mode
    docker run --rm -it --name cliente2 \
        -e CLIENTE_ID=Cliente2 \
        -e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
        cliente2

# Clean up
clean:
    -docker rm -f cliente2 || true
    -docker rm -f partida2 || true
    docker rmi -f cliente2 partida2