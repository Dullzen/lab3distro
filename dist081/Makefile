# Makefile para dist081 (cliente1 y partida1)

all: docker-jugador1-servidor1

# Build cliente1
build-cliente1:
	sudo docker build -t cliente1 ./cliente

# Build partida1
build-partida1:
	sudo docker build -t partida1 ./partidas

# Run both cliente1 and partida1
docker-jugador1-servidor1: build-cliente1 build-partida1
	-sudo docker rm -f cliente1 || true
	-sudo docker rm -f partida1 || true
	# Run partida1 with environment variables
	sudo docker run -d --rm -p 50052:50052 --name partida1 \
		-e PARTIDA_ID=Partida-1 \
		-e PARTIDA_PORT=50052 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		partida1
	# Run cliente1 with interactive mode
	sudo docker run --rm -it --name cliente1 \
		-e CLIENTE_ID=Cliente1 \
		-e MATCHMAKER_ADDRESS=10.35.168.94:50051 \
		cliente1

# Clean up
clean:
	-sudo docker rm -f cliente1 || true
	-sudo docker rm -f partida1 || true
	sudo docker rmi -f cliente1 partida1