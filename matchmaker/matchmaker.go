package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	pb "matchmaker/proto/grpc-server/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Dirección del servidor
const (
	port = ":50051"
)

// Estado de la partida
type EstadoPartida string

const (
	Esperando  EstadoPartida = "Esperando"
	EnCurso    EstadoPartida = "En curso"
	Finalizada EstadoPartida = "Finalizada"
)

// Resultado de una partida
type ResultadoPartida struct {
	Ganador  string
	Perdedor string
}

// Estructura para representar una partida
type Partida struct {
	ID         string
	Clientes   []string
	Estado     EstadoPartida
	Resultado  *ResultadoPartida
	ServidorID string // Añadir campo para el ID del servidor que ejecuta la partida
	mutex      sync.RWMutex
}

// Verificar si la partida está llena
func (p *Partida) EstaLlena() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.Clientes) >= 2
}

// Verificar si la partida está en curso o finalizada
func (p *Partida) EstaActiva() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.Estado == EnCurso || p.Estado == Finalizada
}

// Añadir un cliente a la partida
func (p *Partida) AñadirCliente(clienteID string) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Verificar si el cliente ya está en la partida
	for _, c := range p.Clientes {
		if c == clienteID {
			return false
		}
	}

	// Verificar si hay espacio y la partida está esperando jugadores
	if len(p.Clientes) >= 2 || p.Estado != Esperando {
		return false
	}

	// Añadir el cliente
	p.Clientes = append(p.Clientes, clienteID)

	// Si ahora está llena, cambiar el estado a en curso
	if len(p.Clientes) == 2 {
		p.Estado = EnCurso
	}

	return true
}

// Eliminar un cliente de la partida
func (p *Partida) EliminarCliente(clienteID string) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// No permitir eliminación si la partida está activa
	if p.Estado == EnCurso {
		return false
	}

	for i, c := range p.Clientes {
		if c == clienteID {
			// Eliminar el cliente usando la técnica de reordenamiento
			p.Clientes[i] = p.Clientes[len(p.Clientes)-1]
			p.Clientes = p.Clientes[:len(p.Clientes)-1]
			return true
		}
	}
	return false
}

// Simular el resultado de la partida
func (p *Partida) SimularPartida() *ResultadoPartida {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.Estado != EnCurso || len(p.Clientes) != 2 {
		return nil
	}

	// Elegir un ganador aleatoriamente
	rand.Seed(time.Now().UnixNano())
	ganadorIdx := rand.Intn(2)
	perdedorIdx := (ganadorIdx + 1) % 2

	// Crear el resultado
	p.Resultado = &ResultadoPartida{
		Ganador:  p.Clientes[ganadorIdx],
		Perdedor: p.Clientes[perdedorIdx],
	}

	// Actualizar estado
	p.Estado = Finalizada

	return p.Resultado
}

// Convertir a mensaje protobuf
func (p *Partida) ToProto() *pb.Partida {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	partidaProto := &pb.Partida{
		Id:         p.ID,
		Clientes:   append([]string{}, p.Clientes...), // Crear una copia
		Llena:      len(p.Clientes) >= 2,
		Estado:     string(p.Estado),
		ServidorId: p.ServidorID, // Añadir el ID del servidor a la respuesta
	}

	// Si hay un resultado, incluirlo
	if p.Resultado != nil {
		partidaProto.Ganador = p.Resultado.Ganador
		partidaProto.Perdedor = p.Resultado.Perdedor
	}

	return partidaProto
}

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

// Servidor implementando la interfaz definida en proto
type server struct {
	pb.UnimplementedMatchmakerServer
	pb.UnimplementedMatchmakerServiceServer // Añadir esta línea
	vectorClock                             *VectorClock
	partidas                                map[string]*Partida
	clientePartida                          map[string]string // Mapea clientes a sus partidas
	mutex                                   sync.Mutex
	partidaMutex                            sync.RWMutex

	// Registrar servidor de partidas
	servidoresPartidas map[string]*ServidorPartida // ID del servidor -> info del servidor
	servidoresMutex    sync.RWMutex
}

// Crear nuevas partidas iniciales
func (s *server) crearPartidasIniciales(numPartidas int) {
	s.partidaMutex.Lock()
	defer s.partidaMutex.Unlock()

	for i := 0; i < numPartidas; i++ {
		partidaID := fmt.Sprintf("Partida-%d", i+1)
		s.partidas[partidaID] = &Partida{
			ID:       partidaID,
			Clientes: []string{},
			Estado:   Esperando,
		}
	}
	log.Printf("Creadas %d partidas iniciales", numPartidas)
}

// Obtener partidas disponibles (sólo las que están esperando)
func (s *server) obtenerPartidasDisponibles() []*Partida {
	s.partidaMutex.RLock()
	defer s.partidaMutex.RUnlock()

	var disponibles []*Partida
	for _, p := range s.partidas {
		if !p.EstaLlena() && p.Estado == Esperando {
			disponibles = append(disponibles, p)
		}
	}
	return disponibles
}

// Verificar si un cliente ya está inscrito en alguna partida
func (s *server) clienteYaInscrito(clienteID string) bool {
	s.partidaMutex.RLock()
	defer s.partidaMutex.RUnlock()

	_, existe := s.clientePartida[clienteID]
	return existe
}

// Obtener todas las partidas
func (s *server) obtenerTodasLasPartidas() []*Partida {
	s.partidaMutex.RLock()
	defer s.partidaMutex.RUnlock()

	var todas []*Partida
	for _, p := range s.partidas {
		todas = append(todas, p)
	}
	return todas
}

// Asignar un cliente a una partida aleatoria disponible
func (s *server) asignarClienteAPartida(clienteID string) (string, bool) {
	// Verificar si el cliente ya está inscrito
	if s.clienteYaInscrito(clienteID) {
		log.Printf("Cliente %s ya está inscrito en una partida", clienteID)
		return "", false
	}

	// Obtener partidas realmente disponibles (verificación adicional)
	s.partidaMutex.RLock()
	var disponibles []*Partida
	for _, p := range s.partidas {
		if !p.EstaLlena() && p.Estado == Esperando {
			disponibles = append(disponibles, p)
		}
	}
	s.partidaMutex.RUnlock()

	// Si no hay disponibles, crear nueva
	if len(disponibles) == 0 {
		// Crear una nueva partida si no hay disponibles
		s.partidaMutex.Lock()
		nuevaPartidaID := fmt.Sprintf("Partida-%d", len(s.partidas)+1)
		nuevaPartida := &Partida{
			ID:       nuevaPartidaID,
			Clientes: []string{},
			Estado:   Esperando,
		}
		s.partidas[nuevaPartidaID] = nuevaPartida
		s.partidaMutex.Unlock()

		disponibles = append(disponibles, nuevaPartida)
	}

	// Seleccionar una partida aleatoria de las disponibles
	rand.Seed(time.Now().UnixNano())
	partidaElegida := disponibles[rand.Intn(len(disponibles))]

	// Añadir el cliente a la partida
	if partidaElegida.AñadirCliente(clienteID) {
		s.partidaMutex.Lock()
		s.clientePartida[clienteID] = partidaElegida.ID
		s.partidaMutex.Unlock()

		// Si la partida está llena, iniciar simulación después de un breve periodo
		if partidaElegida.EstaLlena() {
			// Reemplazar esta línea:
			// go s.simularPartidaDespuesDe(partidaElegida.ID, 5*time.Second)

			// Por esta lógica:
			go func(partidaID string) {
				// Buscar un servidor disponible inmediatamente
				servidor, encontrado := s.obtenerServidorPartidaDisponible()
				if !encontrado {
					log.Printf("No hay servidores disponibles para la partida %s. Usando simulación local...", partidaID)
					// Fallback a simulación local después de un breve periodo
					time.Sleep(5 * time.Second)
					s.realizarSimulacionLocal(partidaID)
					return
				}

				log.Printf("Asignando partida %s a servidor externo %s en %s",
					partidaID, servidor.ID, servidor.Address)

				// Obtener los clientes de la partida
				s.partidaMutex.RLock()
				partida, existe := s.partidas[partidaID]
				if !existe || len(partida.Clientes) != 2 {
					s.partidaMutex.RUnlock()
					return
				}
				clientes := append([]string{}, partida.Clientes...)
				s.partidaMutex.RUnlock()

				// Conectar al servidor de partidas
				conn, err := grpc.Dial(
					servidor.Address,
					grpc.WithTransportCredentials(insecure.NewCredentials()),
					grpc.WithBlock(),
					grpc.WithTimeout(3*time.Second),
				)

				// Imprimir información de debug
				log.Printf("Intentando conectar a servidor %s en dirección '%s'",
					servidor.ID, servidor.Address)

				if err != nil {
					log.Printf("Error al conectar con servidor %s: %v", servidor.ID, err)
					s.servidoresMutex.Lock()
					s.servidoresPartidas[servidor.ID].Status = "CAIDO"
					s.servidoresMutex.Unlock()

					// Fallback a simulación local
					time.Sleep(2 * time.Second)
					s.realizarSimulacionLocal(partidaID)
					return
				}
				defer conn.Close()

				// Crear cliente gRPC y enviar solicitud
				client := pb.NewPartidaServiceClient(conn)

				// Incrementar el reloj vectorial
				s.mutex.Lock()
				s.vectorClock.Increment("Matchmaker")
				respClock := s.vectorClock.Get()
				s.mutex.Unlock()

				// Crear solicitud
				req := &pb.AssignMatchRequest{
					MatchId:        partidaID,
					PlayerIds:      clientes,
					RelojVectorial: respClock,
				}

				// Enviar solicitud con timeout
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				resp, err := client.AssignMatch(ctx, req)
				if err != nil {
					log.Printf("ERROR: No se pudo asignar partida %s a servidor %s: %v",
						partidaID, servidor.ID, err)

					// Fallback a simulación local
					time.Sleep(2 * time.Second)
					s.realizarSimulacionLocal(partidaID)
					return
				}

				// Actualizar reloj vectorial con la respuesta
				s.mutex.Lock()
				s.vectorClock.Update(resp.GetRelojVectorial())
				s.mutex.Unlock()

				// Al enviar AssignMatchRequest y recibir respuesta exitosa, actualizar el campo ServidorID
				if err == nil && resp.StatusCode == 0 {
					s.partidaMutex.Lock()
					if partida, existe := s.partidas[partidaID]; existe {
						partida.ServidorID = servidor.ID
						log.Printf("Partida %s asignada y vinculada al servidor %s",
							partidaID, servidor.ID)
					}
					s.partidaMutex.Unlock()
				}

				log.Printf("Partida %s asignada correctamente a servidor %s (código: %d, mensaje: %s)",
					partidaID, servidor.ID, resp.GetStatusCode(), resp.GetMessage())

			}(partidaElegida.ID)
		}

		return partidaElegida.ID, true
	}

	return "", false
}

// Función para obtener un servidor de partidas disponible
func (s *server) obtenerServidorPartidaDisponible() (*ServidorPartida, bool) {
	s.servidoresMutex.RLock()
	defer s.servidoresMutex.RUnlock()

	log.Printf("Buscando servidor de partidas disponible. Servidores registrados: %d", len(s.servidoresPartidas))

	for id, sp := range s.servidoresPartidas {
		log.Printf("Servidor %s tiene estado: %s", id, sp.Status)
		if sp.Status == "DISPONIBLE" {
			log.Printf("Usando servidor disponible: %s en %s", id, sp.Address)
			return sp, true
		}
	}

	log.Printf("No se encontró ningún servidor de partidas disponible")
	return nil, false
}

// Simular una partida después de un tiempo determinado

// Función auxiliar para realizar simulación local
func (s *server) realizarSimulacionLocal(partidaID string) {
	s.partidaMutex.Lock()
	partida, existe := s.partidas[partidaID]

	if !existe || partida.Estado != EnCurso {
		s.partidaMutex.Unlock()
		return
	}

	log.Printf("Iniciando simulación local para partida %s", partidaID)
	resultado := partida.SimularPartida()

	if resultado != nil {
		log.Printf("Partida %s finalizada localmente. Ganador: %s, Perdedor: %s",
			partidaID, resultado.Ganador, resultado.Perdedor)

		// Eliminar a los jugadores de la partida del mapa clientePartida
		for _, cliente := range partida.Clientes {
			delete(s.clientePartida, cliente)
		}
	}

	s.partidaMutex.Unlock()
}

// Eliminar un cliente de su partida
func (s *server) eliminarClienteDePartida(clienteID string) bool {
	s.partidaMutex.RLock()
	partidaID, existe := s.clientePartida[clienteID]
	s.partidaMutex.RUnlock()

	if !existe {
		return false
	}

	s.partidaMutex.Lock()
	partida, existePartida := s.partidas[partidaID]
	s.partidaMutex.Unlock()

	if !existePartida {
		return false
	}

	if partida.EliminarCliente(clienteID) {
		s.partidaMutex.Lock()
		delete(s.clientePartida, clienteID)
		s.partidaMutex.Unlock()
		return true
	}

	return false
}

// Obtener la partida de un cliente
func (s *server) obtenerPartidaDeCliente(clienteID string) (*Partida, bool) {
	s.partidaMutex.RLock()
	defer s.partidaMutex.RUnlock()

	partidaID, existe := s.clientePartida[clienteID]
	if !existe {
		return nil, false
	}

	partida, existePartida := s.partidas[partidaID]
	return partida, existePartida
}

// Implementación del método Conectar
func (s *server) Conectar(ctx context.Context, req *pb.ConexionRequest) (*pb.ConexionResponse, error) {
	mensaje := req.GetMensaje()
	log.Printf("Recibida solicitud: %v", mensaje)

	// Extraer el ID del cliente del mensaje (formato: "ACCIÓN: ClienteID ...")
	partes := strings.Split(mensaje, ":")
	if len(partes) < 2 {
		return nil, fmt.Errorf("formato de mensaje incorrecto")
	}

	accion := strings.TrimSpace(partes[0])
	restoDeMensaje := strings.TrimSpace(partes[1])

	// Extraer el ID del cliente
	clienteID := strings.Split(restoDeMensaje, " ")[0]

	// Incrementar el reloj vectorial del servidor
	s.mutex.Lock()
	s.vectorClock.Increment("Matchmaker")
	s.vectorClock.Update(req.GetRelojVectorial())
	respClock := s.vectorClock.Get()
	s.mutex.Unlock()

	// Preparar respuesta base
	respuesta := &pb.ConexionResponse{
		RelojVectorial: respClock,
		Exito:          true,
	}

	// Procesar según la acción
	switch accion {
	case "INSCRIPCIÓN":
		// Verificar si ya está inscrito
		if s.clienteYaInscrito(clienteID) {
			respuesta.Mensaje = "Ya estás inscrito en una partida. Cancela tu inscripción actual antes de inscribirte nuevamente."
			respuesta.Exito = false
		} else {
			// Intentar asignar a una partida
			partidaID, exito := s.asignarClienteAPartida(clienteID)
			if exito {
				respuesta.Mensaje = fmt.Sprintf("Te has inscrito exitosamente en la partida %s", partidaID)
				respuesta.PartidaId = partidaID
			} else {
				respuesta.Mensaje = "No fue posible inscribirte en una partida"
				respuesta.Exito = false
			}
		}

	case "ESTADO":
		// Convertir todas las partidas a formato protobuf
		var partidasProto []*pb.Partida
		todasPartidas := s.obtenerTodasLasPartidas()
		for _, p := range todasPartidas {
			partidasProto = append(partidasProto, p.ToProto())
		}

		// Verificar si el cliente está en alguna partida
		partida, enPartida := s.obtenerPartidaDeCliente(clienteID)
		if enPartida {
			respuesta.Mensaje = fmt.Sprintf("Estás inscrito en la partida %s (Estado: %s)", partida.ID, partida.Estado)
			respuesta.PartidaId = partida.ID

			// Si la partida finalizó, incluir el resultado
			if partida.Estado == Finalizada && partida.Resultado != nil {
				if partida.Resultado.Ganador == clienteID {
					respuesta.Mensaje = fmt.Sprintf("¡Has GANADO la partida %s contra %s!",
						partida.ID, partida.Resultado.Perdedor)
				} else {
					respuesta.Mensaje = fmt.Sprintf("Has perdido la partida %s contra %s.",
						partida.ID, partida.Resultado.Ganador)
				}
			}
		} else {
			respuesta.Mensaje = "No estás inscrito en ninguna partida"
		}

		respuesta.Partidas = partidasProto

	case "CANCELACIÓN":
		// Solo permitir cancelación si la partida no está en curso
		partida, enPartida := s.obtenerPartidaDeCliente(clienteID)
		if !enPartida {
			respuesta.Mensaje = "No estabas inscrito en ninguna partida"
			respuesta.Exito = false
		} else if partida.Estado == EnCurso {
			respuesta.Mensaje = "No puedes abandonar una partida en curso"
			respuesta.Exito = false
		} else {
			exito := s.eliminarClienteDePartida(clienteID)
			if exito {
				respuesta.Mensaje = "Has sido eliminado de la partida exitosamente"
			} else {
				respuesta.Mensaje = "No se pudo cancelar la inscripción"
				respuesta.Exito = false
			}
		}

	default:
		respuesta.Mensaje = "Acción no reconocida"
		respuesta.Exito = false
	}

	log.Printf("Respuesta: %s", respuesta.Mensaje)
	return respuesta, nil
}

// Implementación del método QueuePlayer
func (s *server) QueuePlayer(ctx context.Context, req *pb.PlayerInfoRequest) (*pb.QueuePlayerResponse, error) {
	playerID := req.GetPlayerId()
	gameMode := req.GetGameModePreference()
	log.Printf("Recibida solicitud de emparejamiento de %s para modo %s", playerID, gameMode)

	// Incrementar el reloj vectorial del servidor
	s.mutex.Lock()
	s.vectorClock.Increment("Matchmaker")
	s.vectorClock.Update(req.GetRelojVectorial())
	respClock := s.vectorClock.Get()
	s.mutex.Unlock()

	// Preparar respuesta base
	respuesta := &pb.QueuePlayerResponse{
		RelojVectorial: respClock,
		StatusCode:     0, // Éxito por defecto
	}

	// Si es una solicitud de cancelación
	if gameMode == "CANCEL" {
		// Solo permitir cancelación si la partida no está en curso
		partida, enPartida := s.obtenerPartidaDeCliente(playerID)
		if !enPartida {
			respuesta.StatusCode = 1 // Error
			respuesta.Message = "No estabas inscrito en ninguna partida"
		} else if partida.Estado == EnCurso {
			respuesta.StatusCode = 2 // Error
			respuesta.Message = "No puedes abandonar una partida en curso"
		} else {
			exito := s.eliminarClienteDePartida(playerID)
			if exito {
				respuesta.Message = "Has sido eliminado de la partida exitosamente"
			} else {
				respuesta.StatusCode = 3 // Error
				respuesta.Message = "No se pudo cancelar la inscripción"
			}
		}
		return respuesta, nil
	}

	// Si es solicitud normal de inscripción
	// Verificar si ya está inscrito
	if s.clienteYaInscrito(playerID) {
		respuesta.StatusCode = 1 // Código de error
		respuesta.Message = "Ya estás inscrito en una partida. Cancela tu inscripción actual antes de inscribirte nuevamente."
		return respuesta, nil
	}

	// Intentar asignar a una partida
	partidaID, exito := s.asignarClienteAPartida(playerID)
	if exito {
		respuesta.Message = fmt.Sprintf("Te has inscrito exitosamente en la partida %s", partidaID)
		respuesta.PartidaId = partidaID
	} else {
		respuesta.StatusCode = 2 // Otro código de error
		respuesta.Message = "No fue posible inscribirte en una partida"
	}

	log.Printf("Respuesta: %s", respuesta.Message)
	return respuesta, nil
}

// Implementación del método GetPlayerStatus
func (s *server) GetPlayerStatus(ctx context.Context, req *pb.PlayerStatusRequest) (*pb.PlayerStatusResponse, error) {
	playerID := req.GetPlayerId()
	log.Printf("Recibida solicitud de estado del jugador %s", playerID)

	// Incrementar el reloj vectorial del servidor
	s.mutex.Lock()
	s.vectorClock.Increment("Matchmaker")
	s.vectorClock.Update(req.GetRelojVectorial())
	respClock := s.vectorClock.Get()
	s.mutex.Unlock()

	// Convertir todas las partidas a formato protobuf
	var partidasProto []*pb.Partida
	todasPartidas := s.obtenerTodasLasPartidas()
	for _, p := range todasPartidas {
		partidasProto = append(partidasProto, p.ToProto())
	}

	// Preparar respuesta base
	respuesta := &pb.PlayerStatusResponse{
		RelojVectorial: respClock,
		PlayerStatus:   "IDLE", // Por defecto está inactivo
		Partidas:       partidasProto,
	}

	// Verificar si el cliente está en alguna partida
	partida, enPartida := s.obtenerPartidaDeCliente(playerID)
	if enPartida {
		// Verificación adicional para evitar referencias a partidas finalizadas
		if partida.Estado == Finalizada {
			log.Printf("ADVERTENCIA: El cliente %s aparece en una partida finalizada %s. Corrigiendo...",
				playerID, partida.ID)
			// Eliminar la asociación incorrecta
			s.partidaMutex.Lock()
			delete(s.clientePartida, playerID)
			s.partidaMutex.Unlock()

			enPartida = false
			respuesta.Mensaje = "No estás inscrito en ninguna partida"
			respuesta.PlayerStatus = "IDLE"
		} else {
			// Si está en una partida activa, continuar normalmente
			switch partida.Estado {
			case Esperando:
				respuesta.PlayerStatus = "IN_QUEUE"
				respuesta.Mensaje = fmt.Sprintf("Estás inscrito en la partida %s (Estado: %s)", partida.ID, partida.Estado)
			case EnCurso:
				respuesta.PlayerStatus = "IN_MATCH"
				respuesta.Mensaje = fmt.Sprintf("Estás participando en la partida %s", partida.ID)
			}
			respuesta.PartidaId = partida.ID
		}
	} else {
		respuesta.Mensaje = "No estás inscrito en ninguna partida"
	}

	log.Printf("Respuesta: %s", respuesta.Mensaje)
	return respuesta, nil
}

// Implementación del método SincronizarReloj
func (s *server) SincronizarReloj(ctx context.Context, req *pb.SincronizacionRequest) (*pb.SincronizacionResponse, error) {
	clienteID := req.GetIdCliente()
	log.Printf("Recibida solicitud de sincronización de %s", clienteID)

	// Incrementar el reloj vectorial del servidor
	s.mutex.Lock()
	s.vectorClock.Increment("Matchmaker")
	s.vectorClock.Update(req.GetRelojVectorial())
	respClock := s.vectorClock.Get()
	s.mutex.Unlock()

	log.Printf("Reloj vectorial sincronizado con %s: %v", clienteID, respClock)

	return &pb.SincronizacionResponse{
		RelojVectorial: respClock,
		Exito:          true,
	}, nil
}

// Implementación del método UpdateServerStatus
func (s *server) UpdateServerStatus(ctx context.Context, req *pb.ServerStatusUpdateRequest) (*pb.ServerStatusUpdateResponse, error) {
	serverID := req.GetServerId()
	status := req.GetStatus()
	address := req.GetAddress()

	log.Printf("Recibida actualización de estado del servidor de partidas %s: %s en %s",
		serverID, status, address)

	// Incrementar el reloj vectorial del servidor
	s.mutex.Lock()
	s.vectorClock.Increment("Matchmaker")
	s.vectorClock.Update(req.GetRelojVectorial())
	respClock := s.vectorClock.Get()
	s.mutex.Unlock()

	// Actualizar registro del servidor de partidas
	s.servidoresMutex.Lock()
	if _, existe := s.servidoresPartidas[serverID]; !existe {
		log.Printf("Nuevo servidor de partidas registrado: %s", serverID)
	}

	s.servidoresPartidas[serverID] = &ServidorPartida{
		ID:         serverID,
		Address:    address,
		Status:     status,
		LastUpdate: time.Now(),
	}
	s.servidoresMutex.Unlock()

	log.Printf("Servidor %s registrado/actualizado con dirección '%s' y estado '%s'",
		serverID, address, status)

	return &pb.ServerStatusUpdateResponse{
		StatusCode:     0,
		Message:        "Estado actualizado correctamente",
		RelojVectorial: respClock,
	}, nil
}

// Estructura para representar un servidor de partidas
type ServidorPartida struct {
	ID         string
	Address    string
	Status     string
	LastUpdate time.Time
}

// Implementación del método NotifyMatchResult
func (s *server) NotifyMatchResult(ctx context.Context, req *pb.MatchResultNotification) (*pb.MatchResultResponse, error) {
	matchID := req.GetMatchId()
	ganadorID := req.GetWinnerId()
	perdedorID := req.GetLoserId()
	serverID := req.GetServerId()

	log.Printf("Recibida notificación de resultado de partida %s desde servidor %s: Ganador=%s, Perdedor=%s",
		matchID, serverID, ganadorID, perdedorID)

	// Incrementar el reloj vectorial del servidor
	s.mutex.Lock()
	s.vectorClock.Increment("Matchmaker")
	s.vectorClock.Update(req.GetRelojVectorial())
	respClock := s.vectorClock.Get()
	s.mutex.Unlock()

	// Actualizar el estado de la partida en el matchmaker
	s.partidaMutex.Lock()
	partida, existe := s.partidas[matchID]
	if !existe {
		s.partidaMutex.Unlock()
		return &pb.MatchResultResponse{
			StatusCode:     1,
			Message:        "Partida no encontrada",
			RelojVectorial: respClock,
		}, nil
	}

	// Actualizar estado y resultado
	partida.Estado = Finalizada
	partida.Resultado = &ResultadoPartida{
		Ganador:  ganadorID,
		Perdedor: perdedorID,
	}

	// Eliminar las asociaciones cliente-partida
	for _, cliente := range partida.Clientes {
		delete(s.clientePartida, cliente)
	}

	// Importante: marcar explícitamente que la partida ya no está disponible
	// para evitar que sea reutilizada inmediatamente
	partidaID := partida.ID

	// Opcionalmente, renombrar la partida para evitar colisiones
	nuevoID := fmt.Sprintf("%s-completed-%d", partidaID, time.Now().Unix())
	s.partidas[nuevoID] = partida
	delete(s.partidas, partidaID)

	s.partidaMutex.Unlock()

	// Crear nueva partida disponible con el ID original
	s.partidaMutex.Lock()
	s.partidas[partidaID] = &Partida{
		ID:       partidaID,
		Clientes: []string{},
		Estado:   Esperando,
	}
	s.partidaMutex.Unlock()

	log.Printf("Resultado de partida %s registrado y creada nueva partida con ID %s", nuevoID, partidaID)

	return &pb.MatchResultResponse{
		StatusCode:     0,
		Message:        "Resultado registrado correctamente",
		RelojVectorial: respClock,
	}, nil
}

func main() {
	// Inicializar el generador de números aleatorios
	rand.Seed(time.Now().UnixNano())

	// Crear listener TCP en el puerto especificado
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Error al escuchar: %v", err)
	}

	// Crear servidor gRPC
	s := &server{
		vectorClock:        NewVectorClock("Matchmaker"),
		partidas:           make(map[string]*Partida),
		clientePartida:     make(map[string]string),
		servidoresPartidas: make(map[string]*ServidorPartida),
	}

	// Crear algunas partidas iniciales
	s.crearPartidasIniciales(3)

	grpcServer := grpc.NewServer()

	// Registrar el servicio de Matchmaker en el servidor
	pb.RegisterMatchmakerServer(grpcServer, s)

	// También registrar el servicio que usan los servidores de partidas
	pb.RegisterMatchmakerServiceServer(grpcServer, s)

	fmt.Printf("Servidor matchmaker iniciado en %s\n", port)

	// Iniciar servidor
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Error al servir: %v", err)
	}
}
