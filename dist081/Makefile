# Makefile para dist081 (cliente1 y partida1)

all: docker-jugador1-servidor1

# Build cliente1
build-cliente1:
	sudo docker build -t cliente1 ./cliente

# Build partida1
build-partida1:
	sudo docker build -t partida1 ./partidas

# Run partida1 in a separate terminal with logs visible
run-partida1:
	-sudo docker rm -f partida1 || true
	sudo docker run --rm -p 50052:50052 --name partida1 \
		-e PARTIDA_ID=Partida-1 \
		-e PARTIDA_PORT=50052 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		-e LOG_PREFIX="[Partida-1]" \
		partida1

# Run cliente1 in a separate terminal
run-cliente1:
	-sudo docker rm -f cliente1 || true
	sudo docker run --rm -it --name cliente1 \
		-e CLIENTE_ID=Cliente1 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		cliente1

# Run both in background, then show partida logs and cliente in interactive mode
docker-jugador1-servidor1: build-cliente1 build-partida1
	-sudo docker rm -f cliente1 || true
	-sudo docker rm -f partida1 || true
	sudo docker run -d --rm -p 50052:50052 --name partida1 \
		-e PARTIDA_ID=Partida-1 \
		-e PARTIDA_PORT=50052 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		-e LOG_PREFIX="[Partida-1]" \
		partida1
	sudo docker run --rm -it --name cliente1 \
		-e CLIENTE_ID=Cliente1 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		cliente1

# Clean up
clean:
	-sudo docker rm -f cliente1 || true
	-sudo docker rm -f partida1 || true
	sudo docker rmi -f cliente1 partida1

# View logs for partida1
logs-partida:
	sudo docker logs -f partida1