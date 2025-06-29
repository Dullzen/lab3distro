# Makefile para dist081 (cliente1 y partida1)

all: docker-jugador1-servidor1

# Build cliente1
build-cliente1:
	docker build -t cliente1 ../cliente

# Build partida1
build-partida1:
	docker build -t partida1 ../partidas

# Run both cliente1 and partida1
docker-jugador1-servidor1: build-cliente1 build-partida1
	-docker rm -f cliente1 || true
	-docker rm -f partida1 || true
	# Run partida1 with environment variables
	docker run -d --rm -p 50052:50052 --name partida1 \
		-e PARTIDA_ID=Partida-1 \
		-e PARTIDA_PORT=50052 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		partida1
	# Run cliente1 with interactive mode
	docker run --rm -it --name cliente1 \
		-e CLIENTE_ID=Cliente1 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		cliente1

# Clean up
clean:
	-docker rm -f cliente1 || true
	-docker rm -f partida1 || true
	docker rmi -f cliente1 partida1