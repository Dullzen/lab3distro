package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	pb "cliente/proto/grpc-server/proto"

	"google.golang.org/grpc"
)

const (
	// Dirección del servidor matchmaker
	matchmakerAddr = "matchmaker:50051"
)

// Estructura para manejar el reloj vectorial
type VectorClock struct {
	clock map[string]int32
	mutex sync.RWMutex
}

// Crear un nuevo reloj vectorial
func NewVectorClock(id string) *VectorClock {
	vc := &VectorClock{
		clock: make(map[string]int32),
	}
	vc.clock[id] = 0
	return vc
}

// Incrementar el contador propio
func (vc *VectorClock) Increment(id string) {
	vc.mutex.Lock()
	defer vc.mutex.Unlock()
	vc.clock[id]++
}

// Obtener una copia del reloj actual
func (vc *VectorClock) Get() map[string]int32 {
	vc.mutex.RLock()
	defer vc.mutex.RUnlock()

	// Crear una copia para evitar problemas de concurrencia
	copy := make(map[string]int32)
	for k, v := range vc.clock {
		copy[k] = v
	}
	return copy
}

// Actualizar el reloj vectorial con otro reloj
func (vc *VectorClock) Update(other map[string]int32) {
	vc.mutex.Lock()
	defer vc.mutex.Unlock()

	// Incorporar todos los valores del otro reloj
	for id, value := range other {
		// Si no existe o el otro tiene un valor mayor, actualizamos
		if currentValue, exists := vc.clock[id]; !exists || value > currentValue {
			vc.clock[id] = value
		}
	}
}

// Función para conectar con reintentos
func conectarGRPC(address string, maxRetries int, retryDelay time.Duration) (*grpc.ClientConn, error) {
	var conn *grpc.ClientConn
	var err error

	for i := 0; i < maxRetries; i++ {
		conn, err = grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(3*time.Second))
		if err == nil {
			return conn, nil // Conexión exitosa
		}

		log.Printf("Intento %d: No se pudo conectar a %s: %v", i+1, address, err)
		time.Sleep(retryDelay) // Esperar antes de reintentar
	}

	return nil, fmt.Errorf("no se pudo conectar a %s después de %d intentos", address, maxRetries)
}

// Función para mostrar el menú y obtener la opción del usuario
func mostrarMenu(clienteID string) int {
	fmt.Printf("\n===== %s - MENÚ =====\n", clienteID)
	fmt.Println("1. Inscribirse para emparejamiento")
	fmt.Println("2. Solicitar estado actual")
	fmt.Println("3. Cancelar emparejamiento")
	fmt.Println("0. Salir")
	fmt.Print("\nSeleccione una opción: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var opcion int
	fmt.Sscanf(input, "%d", &opcion)
	return opcion
}

// Función para mostrar la información de partidas
func mostrarPartidas(partidas []*pb.Partida) {
	if len(partidas) == 0 {
		fmt.Println("No hay partidas disponibles")
		return
	}

	fmt.Println("\n=== LISTADO DE PARTIDAS ===")

	// Crear un mapa para organizar partidas por servidor y por ID
	partidasPorServidor := make(map[string]*pb.Partida)
	partidasPorID := make(map[string]*pb.Partida)

	// Organizar partidas tanto por servidor como por ID
	for _, p := range partidas {
		partidasPorID[p.Id] = p

		// Si tiene ServidorId, también guardar referencia
		if p.ServidorId != "" {
			partidasPorServidor[p.ServidorId] = p
		}
	}

	// Ordenar IDs para mostrar siempre en el mismo orden
	servidores := []string{"Partida-1", "Partida-2", "Partida-3"}

	// Mostrar partidas según el orden de servidores
	for i, servidorID := range servidores {
		var p *pb.Partida
		var partidaInfo string

		// Revisar si hay una partida ejecutándose en este servidor
		partidaServidor, existeEnServidor := partidasPorServidor[servidorID]

		// También revisar si hay una partida con este ID
		partidaID, existePorID := partidasPorID[servidorID]

		// Priorizar partida que se ejecuta en el servidor sobre partida que solo tiene ese ID
		if existeEnServidor {
			p = partidaServidor

			estado := p.Estado
			if estado == "" {
				estado = "Esperando"
			}

			status := "Disponible"
			if p.Llena {
				status = "Llena"
			}

			partidaInfo = fmt.Sprintf("%d. %s (Estado: %s, %s) - Jugadores: %s",
				i+1, servidorID, estado, status, strings.Join(p.Clientes, ", "))

			if estado == "Finalizada" && p.Ganador != "" {
				partidaInfo += fmt.Sprintf(" | Ganador: %s, Perdedor: %s", p.Ganador, p.Perdedor)
			}
		} else if existePorID {
			p = partidaID

			estado := p.Estado
			if estado == "" {
				estado = "Esperando"
			}

			status := "Disponible"
			if p.Llena {
				status = "Llena"
			}

			partidaInfo = fmt.Sprintf("%d. %s (Estado: %s, %s) - Jugadores: %s",
				i+1, servidorID, estado, status, strings.Join(p.Clientes, ", "))

			if estado == "Finalizada" && p.Ganador != "" {
				partidaInfo += fmt.Sprintf(" | Ganador: %s, Perdedor: %s", p.Ganador, p.Perdedor)
			}
		} else {
			// Si no hay partida para este servidor, mostrar un placeholder
			partidaInfo = fmt.Sprintf("%d. %s (Estado: No disponible) - Jugadores:", i+1, servidorID)
		}

		fmt.Println(partidaInfo)
	}
	fmt.Println("========================")
}

// Actualizar las funciones para usar la nueva API

// Función para unirse a la cola de emparejamiento
func queuePlayer(client pb.MatchmakerClient, clienteID string, vectorClock *VectorClock, detenerConsulta *bool) {
	// Incrementar el reloj vectorial antes de enviar un mensaje
	vectorClock.Increment(clienteID)

	// Contexto con timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Crear solicitud
	req := &pb.PlayerInfoRequest{
		PlayerId:           clienteID,
		GameModePreference: "standard", // Modo por defecto
		RelojVectorial:     vectorClock.Get(),
	}

	// Enviar solicitud
	resp, err := client.QueuePlayer(ctx, req)

	if err != nil {
		log.Printf("[%s] Error en la comunicación: %v", clienteID, err)
		return
	}

	// Actualizar nuestro reloj vectorial con la respuesta
	vectorClock.Update(resp.RelojVectorial)

	// Mostrar respuesta
	if resp.StatusCode == 0 {
		fmt.Printf("[%s] ✅ %s\n", clienteID, resp.Message)
	} else {
		fmt.Printf("[%s] ❌ %s\n", clienteID, resp.Message)
	}

	// Si nos asignaron una partida, mostrar el ID
	if resp.PartidaId != "" {
		fmt.Printf("[%s] Partida asignada: %s\n", clienteID, resp.PartidaId)
	}

	fmt.Printf("[%s] Reloj vectorial actual: %v\n", clienteID, vectorClock.Get())
}

// Función para consultar el estado del jugador
func getPlayerStatus(client pb.MatchmakerClient, clienteID string, vectorClock *VectorClock, detenerConsulta *bool) {
	// Incrementar el reloj vectorial antes de enviar un mensaje
	vectorClock.Increment(clienteID)

	// Contexto con timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Crear solicitud
	req := &pb.PlayerStatusRequest{
		PlayerId:       clienteID,
		RelojVectorial: vectorClock.Get(),
	}

	// Enviar solicitud
	resp, err := client.GetPlayerStatus(ctx, req)

	if err != nil {
		log.Printf("[%s] Error en la comunicación: %v", clienteID, err)
		return
	}

	// Actualizar nuestro reloj vectorial con la respuesta
	vectorClock.Update(resp.RelojVectorial)

	// Mostrar respuesta
	fmt.Printf("[%s] Estado actual: %s\n", clienteID, resp.PlayerStatus)
	fmt.Printf("[%s] %s\n", clienteID, resp.Mensaje)

	// Si recibimos información de partidas, mostrarla
	if len(resp.Partidas) > 0 {
		mostrarPartidas(resp.Partidas)
	}

	// Si estamos en una partida, mostrar el ID
	if resp.PartidaId != "" {
		// Nueva lógica para mostrar el servidor si es necesario
		partidaAsignada := resp.GetPartidaId()
		partidaServidor := ""

		// Buscar en la lista de partidas para encontrar el servidor real
		for _, p := range resp.GetPartidas() {
			if p.Id == partidaAsignada {
				// Si la partida tiene un servidor asignado, usar ese ID
				if p.ServidorId != "" {
					partidaServidor = p.ServidorId
				}
				break
			}
		}

		// Mostrar información de la partida asignada
		if resp.PlayerStatus == "IN_MATCH" || resp.PlayerStatus == "IN_QUEUE" {
			// Si hay un servidor específico, mostrarlo
			if partidaServidor != "" && partidaServidor != partidaAsignada {
				fmt.Printf("[%s] Asignado a partida lógica: %s (ejecutándose en servidor: %s)\n",
					clienteID, partidaAsignada, partidaServidor)
			} else {
				fmt.Printf("[%s] Partida asignada: %s\n", clienteID, partidaAsignada)
			}
		}
	}

	// Detener las consultas periódicas si:
	// 1. La partida ha finalizado (MATCH_COMPLETED)
	// 2. O el estado es IDLE y el cliente aparece en alguna partida finalizada
	if detenerConsulta != nil {
		if resp.PlayerStatus == "MATCH_COMPLETED" {
			fmt.Printf("[%s] Deteniendo consultas automáticas debido a finalización.\n", clienteID)
			*detenerConsulta = true
			return
		}

		// Verificar si el cliente está en alguna partida finalizada a pesar de estar IDLE
		if resp.PlayerStatus == "IDLE" {
			// Buscar al cliente en partidas finalizadas
			for _, p := range resp.Partidas {
				if p.Estado == "Finalizada" {
					for _, c := range p.Clientes {
						if c == clienteID {
							fmt.Printf("[%s] Deteniendo consultas automáticas. Cliente encontrado en partida finalizada.\n", clienteID)
							*detenerConsulta = true
							return
						}
					}
				}
			}
		}
	}

	fmt.Printf("[%s] Reloj vectorial actual: %v\n", clienteID, vectorClock.Get())
}

// Función para consultar periódicamente el estado del jugador
func consultarEstadoPeriodicamente(client pb.MatchmakerClient, clienteID string, vectorClock *VectorClock, segundos int, detener *bool) {
	for i := 0; i < segundos && !*detener; i++ {
		time.Sleep(5 * time.Second) // Consultar cada 5 segundos

		// Verificar nuevamente para salir rápido si se ha solicitado detener
		if *detener {
			fmt.Printf("[%s] Deteniendo consultas periódicas según lo solicitado.\n", clienteID)
			return
		}

		fmt.Printf("\n[%s] Consultando estado automáticamente...\n", clienteID)

		// Consultar estado
		getPlayerStatus(client, clienteID, vectorClock, detener)
	}
}

// Función para enviar una solicitud de desinscripción (cancelación)
func cancelQueuePlayer(client pb.MatchmakerClient, clienteID string, vectorClock *VectorClock, detenerConsulta *bool) {
	// Esta función podría ser implementada como un nuevo método en la API,
	// pero para compatibilidad, usaremos GetPlayerStatus con un manejo especial

	// Incrementar el reloj vectorial antes de enviar un mensaje
	vectorClock.Increment(clienteID)

	// Contexto con timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Crear solicitud como un mensaje especial (podría ser mejorado)
	req := &pb.PlayerInfoRequest{
		PlayerId:           clienteID,
		GameModePreference: "CANCEL", // Indicador especial
		RelojVectorial:     vectorClock.Get(),
	}

	// Enviar solicitud
	resp, err := client.QueuePlayer(ctx, req)

	if err != nil {
		log.Printf("[%s] Error en la comunicación: %v", clienteID, err)
		return
	}

	// Actualizar nuestro reloj vectorial con la respuesta
	vectorClock.Update(resp.RelojVectorial)

	// Mostrar respuesta
	if resp.StatusCode == 0 {
		fmt.Printf("[%s] ✅ Emparejamiento cancelado exitosamente\n", clienteID)
	} else {
		fmt.Printf("[%s] ❌ %s\n", clienteID, resp.Message)
	}

	// Detener consulta periódica
	if detenerConsulta != nil {
		*detenerConsulta = true
	}

	fmt.Printf("[%s] Reloj vectorial actual: %v\n", clienteID, vectorClock.Get())
}

// Función principal
func main() {
	// Obtener ID del cliente desde variable de entorno
	clienteID := os.Getenv("CLIENTE_ID")
	if clienteID == "" {
		clienteID = "ClienteDesconocido"
	}
	log.Printf("Iniciando %s", clienteID)

	// Inicializar el reloj vectorial para este cliente
	vectorClock := NewVectorClock(clienteID)

	// Configurar conexión gRPC con reintentos
	conn, err := conectarGRPC(matchmakerAddr, 5, 2*time.Second)
	if err != nil {
		log.Fatalf("[%s] Error al conectar: %v", clienteID, err)
	}
	defer conn.Close()

	log.Printf("[%s] Conexión establecida con %s", clienteID, matchmakerAddr)

	// Crear cliente gRPC
	client := pb.NewMatchmakerClient(conn)

	// Variables para control de consulta periódica
	consultando := false
	detenerConsulta := false

	// Bucle principal del menú
	for {
		opcion := mostrarMenu(clienteID)

		switch opcion {
		case 1:
			fmt.Printf("[%s] Enviando solicitud de inscripción...\n", clienteID)
			queuePlayer(client, clienteID, vectorClock, &detenerConsulta)

			// Iniciar consulta periódica si no está activa
			if !consultando {
				consultando = true
				detenerConsulta = false
				go consultarEstadoPeriodicamente(client, clienteID, vectorClock, 12, &detenerConsulta)
			}

		case 2:
			fmt.Printf("[%s] Solicitando estado actual...\n", clienteID)
			getPlayerStatus(client, clienteID, vectorClock, &detenerConsulta)

			// Si estamos consultando periódicamente y recibimos MATCH_COMPLETED,
			// actualizar también la variable de control
			if consultando && detenerConsulta {
				consultando = false
			}

		case 3:
			fmt.Printf("[%s] Cancelando emparejamiento...\n", clienteID)
			cancelQueuePlayer(client, clienteID, vectorClock, &detenerConsulta)

			// Detener consulta periódica si está activa
			if consultando {
				detenerConsulta = true
				consultando = false
			}

		case 0:
			fmt.Printf("[%s] Saliendo...\n", clienteID)
			// Detener consulta periódica si está activa
			if consultando {
				detenerConsulta = true
			}
			return

		default:
			fmt.Println("Opción no válida. Por favor, intente de nuevo.")
		}

		time.Sleep(1 * time.Second)
	}
}
