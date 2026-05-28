package exam

import (
	"math"
	"strconv"
)

// Clave de respuestas y baremo del examen de Variable Compleja. Esta es la
// única fuente de verdad para la calificación; el frontend nunca envía la nota.
const (
	p1StepCount = 8
	p1PerStep   = 0.25 // 8 pasos * 0.25 = 2.00

	p2PerInciso = 0.50 // 4 incisos * 0.50 = 2.00

	p3Points    = 1.00
	p3CorrectID = "p3_1"
)

var p2Incisos = []string{"a", "b", "c", "d"}

// Grade calcula la nota a partir de las respuestas crudas.
func Grade(a AnswersPayload) GradeResult {
	q1 := gradeP1(a.P1)
	q2 := gradeP2(a.P2)
	q3 := gradeP3(a.P3)

	return GradeResult{
		Score:     round2(q1.Points + q2.Points + q3.Points),
		MaxScore:  round2(q1.Max + q2.Max + q3.Max),
		Questions: []QuestionResult{q1, q2, q3},
	}
}

func gradeP1(selected []string) QuestionResult {
	q := QuestionResult{
		Question: "Pregunta 1",
		Title:    "Raíz de un número complejo",
		Max:      round2(p1StepCount * p1PerStep),
		Items:    make([]ItemResult, 0, p1StepCount),
	}

	for i := 0; i < p1StepCount; i++ {
		sel := ""
		if i < len(selected) {
			sel = selected[i]
		}

		correct := sel == strconv.Itoa(i)
		points := 0.0
		if correct {
			points = p1PerStep
			q.Points += p1PerStep
		}

		label := "Procedimiento " + strconv.Itoa(i+1)
		q.Items = append(q.Items, ItemResult{
			Label:    label,
			Selected: describeP1Selection(sel),
			Correct:  correct,
			Points:   points,
			Max:      p1PerStep,
		})
	}

	q.Points = round2(q.Points)
	return q
}

func describeP1Selection(sel string) string {
	if sel == "" {
		return "Sin responder"
	}
	if _, err := strconv.Atoi(sel); err == nil {
		return "Procedimiento " + sel + " (correcto)"
	}
	return "Procedimiento incorrecto (distractor)"
}

func gradeP2(selected map[string]string) QuestionResult {
	q := QuestionResult{
		Question: "Pregunta 2",
		Title:    "Derivada de una función compleja",
		Max:      round2(float64(len(p2Incisos)) * p2PerInciso),
		Items:    make([]ItemResult, 0, len(p2Incisos)),
	}

	for _, inciso := range p2Incisos {
		sel := ""
		if selected != nil {
			sel = selected[inciso]
		}

		correct := sel == "op_"+inciso
		points := 0.0
		if correct {
			points = p2PerInciso
			q.Points += p2PerInciso
		}

		seleccion := "Sin responder"
		if sel != "" {
			seleccion = "Opción " + sel
		}

		q.Items = append(q.Items, ItemResult{
			Label:    "Inciso " + inciso + ")",
			Selected: seleccion,
			Correct:  correct,
			Points:   points,
			Max:      p2PerInciso,
		})
	}

	q.Points = round2(q.Points)
	return q
}

func gradeP3(selected string) QuestionResult {
	correct := selected == p3CorrectID
	points := 0.0
	if correct {
		points = p3Points
	}

	seleccion := "Sin responder"
	if selected != "" {
		seleccion = "Opción " + selected
	}

	return QuestionResult{
		Question: "Pregunta 3",
		Title:    "Integral de variable compleja",
		Points:   round2(points),
		Max:      p3Points,
		Items: []ItemResult{{
			Label:    "Respuesta única",
			Selected: seleccion,
			Correct:  correct,
			Points:   points,
			Max:      p3Points,
		}},
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
