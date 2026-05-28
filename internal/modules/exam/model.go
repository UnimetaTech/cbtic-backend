package exam

import "time"

// Identificador del examen. Permite que en el futuro convivan varios exámenes
// en la misma tabla sin colisionar el "un solo intento por estudiante".
const ExamVariableComplejaID = "variable_compleja_final"

// Estados con los que puede cerrarse un examen.
const (
	StatusCompleted = "completed" // terminó todas las preguntas
	StatusManual    = "manual"    // el estudiante pulsó "Terminar examen"
	StatusTimeout   = "timeout"   // se acabó el tiempo
	StatusFraud     = "fraud"     // anulado por salir de la ventana / atajos
)

func IsValidStatus(s string) bool {
	switch s {
	case StatusCompleted, StatusManual, StatusTimeout, StatusFraud:
		return true
	default:
		return false
	}
}

// AnswersPayload son las respuestas crudas que envía el frontend. NO contiene
// la nota: el backend la calcula a partir de estos identificadores (la clave de
// respuestas vive solo en el servidor), de modo que no se pueda falsear.
type AnswersPayload struct {
	// P1: id seleccionado en cada paso (índice = posición del paso 0..7).
	// Los procedimientos correctos tienen id numérico "0".."7"; los
	// distractores tienen id "d1".."d4".
	P1 []string `json:"p1"`
	// P2: inciso -> id de la opción elegida (op_a..op_p). La opción correcta
	// del inciso x es "op_"+x.
	P2 map[string]string `json:"p2"`
	// P3: id de la opción elegida (p3_1..p3_6). La correcta es p3_1.
	P3 string `json:"p3"`
}

// SubmitRequest es el cuerpo del POST de finalización del examen.
type SubmitRequest struct {
	Email   string         `json:"email"`
	Status  string         `json:"status"`
	Answers AnswersPayload `json:"answers"`
}

type ItemResult struct {
	Label    string  `json:"label"`
	Selected string  `json:"selected"`
	Correct  bool    `json:"correct"`
	Points   float64 `json:"points"`
	Max      float64 `json:"max"`
}

type QuestionResult struct {
	Question string       `json:"question"`
	Title    string       `json:"title"`
	Points   float64      `json:"points"`
	Max      float64      `json:"max"`
	Items    []ItemResult `json:"items"`
}

type GradeResult struct {
	Score     float64          `json:"score"`
	MaxScore  float64          `json:"maxScore"`
	Questions []QuestionResult `json:"questions"`
}

type SubmitResponse struct {
	Email     string      `json:"email"`
	Status    string      `json:"status"`
	Grade     GradeResult `json:"grade"`
	EmailSent bool        `json:"emailSent"`
	Message   string      `json:"message"`
}

type StatusResponse struct {
	Submitted bool `json:"submitted"`
}

type Submission struct {
	ID        string
	ExamID    string
	Email     string
	Score     float64
	MaxScore  float64
	Status    string
	CreatedAt time.Time
}
