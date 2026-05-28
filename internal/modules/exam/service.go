package exam

import (
	"context"
	"fmt"
	"html"
	"log"
	"strings"
)

type Service struct {
	repo         *Repository
	mailer       *Mailer
	teacherEmail string
}

func NewService(repo *Repository, mailer *Mailer, teacherEmail string) *Service {
	return &Service{
		repo:         repo,
		mailer:       mailer,
		teacherEmail: strings.TrimSpace(teacherEmail),
	}
}

func (s *Service) AlreadySubmitted(ctx context.Context, examID, email string) (bool, error) {
	return s.repo.HasSubmitted(ctx, examID, email)
}

type SubmitInput struct {
	ExamID    string
	Email     string
	Status    string
	Answers   AnswersPayload
	ClientIP  string
	UserAgent string
}

// Submit califica (server-side), persiste con barrera de intento único y envía
// el correo al docente y al estudiante.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*SubmitResponse, error) {
	grade := Grade(in.Answers)

	if _, err := s.repo.Insert(ctx, InsertParams{
		ExamID:    in.ExamID,
		Email:     in.Email,
		Score:     grade.Score,
		MaxScore:  grade.MaxScore,
		Status:    in.Status,
		Answers:   in.Answers,
		Breakdown: grade,
		ClientIP:  in.ClientIP,
		UserAgent: in.UserAgent,
	}); err != nil {
		return nil, err
	}

	emailSent := s.notify(in.Email, in.Status, grade)

	message := "Examen registrado correctamente."
	if !emailSent {
		message = "Examen registrado, pero no se pudo enviar el correo de notificación."
	}

	return &SubmitResponse{
		Email:     in.Email,
		Status:    in.Status,
		Grade:     grade,
		EmailSent: emailSent,
		Message:   message,
	}, nil
}

// notify envía el resultado al docente y al estudiante. Es best-effort: si el
// correo falla, la presentación ya quedó registrada. Devuelve true si al menos
// el correo del docente se envió.
func (s *Service) notify(studentEmail, status string, grade GradeResult) bool {
	if !s.mailer.Enabled() {
		log.Printf("exam: SMTP no configurado, se omite el envío de correo")
		return false
	}

	teacherOK := true
	if s.teacherEmail != "" {
		subject := fmt.Sprintf("Examen Variable Compleja — %s — %.2f/%.2f", studentEmail, grade.Score, grade.MaxScore)
		body := buildEmailHTML(studentEmail, status, grade, true)
		if err := s.mailer.SendHTML(s.teacherEmail, subject, body); err != nil {
			log.Printf("exam: error enviando correo al docente (%s): %v", s.teacherEmail, err)
			teacherOK = false
		}
	}

	subject := "Tu resultado del examen de Variable Compleja"
	body := buildEmailHTML(studentEmail, status, grade, false)
	if err := s.mailer.SendHTML(studentEmail, subject, body); err != nil {
		log.Printf("exam: error enviando correo al estudiante (%s): %v", studentEmail, err)
	}

	return teacherOK
}

func buildEmailHTML(studentEmail, status string, grade GradeResult, forTeacher bool) string {
	var b strings.Builder

	b.WriteString(`<div style="font-family:Arial,sans-serif;color:#1f2937;max-width:640px;margin:auto">`)
	b.WriteString(`<h1 style="color:#1e3a8a">Examen final de Variable Compleja</h1>`)

	if forTeacher {
		b.WriteString(`<p>Se registró una nueva presentación del examen.</p>`)
	} else {
		b.WriteString(`<p>Este es el comprobante de tu presentación del examen.</p>`)
	}

	b.WriteString(`<p><strong>Estudiante:</strong> ` + html.EscapeString(studentEmail) + `<br>`)
	b.WriteString(`<strong>Estado:</strong> ` + html.EscapeString(statusLabel(status)) + `</p>`)

	b.WriteString(`<table style="width:100%;border-collapse:collapse;margin:16px 0">`)
	b.WriteString(`<thead><tr style="background:#dbeafe;color:#1e3a8a">` +
		`<th style="border:1px solid #9ca3af;padding:8px;text-align:left">Pregunta</th>` +
		`<th style="border:1px solid #9ca3af;padding:8px;text-align:left">Contenido</th>` +
		`<th style="border:1px solid #9ca3af;padding:8px;text-align:right">Puntaje</th>` +
		`<th style="border:1px solid #9ca3af;padding:8px;text-align:right">Máximo</th></tr></thead><tbody>`)

	for _, q := range grade.Questions {
		b.WriteString(`<tr>` +
			`<td style="border:1px solid #9ca3af;padding:8px">` + html.EscapeString(q.Question) + `</td>` +
			`<td style="border:1px solid #9ca3af;padding:8px">` + html.EscapeString(q.Title) + `</td>` +
			fmt.Sprintf(`<td style="border:1px solid #9ca3af;padding:8px;text-align:right">%.2f</td>`, q.Points) +
			fmt.Sprintf(`<td style="border:1px solid #9ca3af;padding:8px;text-align:right">%.2f</td>`, q.Max) +
			`</tr>`)
	}

	b.WriteString(`<tr style="background:#fef3c7;font-weight:bold">` +
		`<td colspan="2" style="border:1px solid #9ca3af;padding:8px">Total</td>` +
		fmt.Sprintf(`<td style="border:1px solid #9ca3af;padding:8px;text-align:right">%.2f</td>`, grade.Score) +
		fmt.Sprintf(`<td style="border:1px solid #9ca3af;padding:8px;text-align:right">%.2f</td>`, grade.MaxScore) +
		`</tr></tbody></table>`)

	b.WriteString(`<h2 style="color:#1e3a8a;font-size:18px">Detalle de respuestas</h2>`)
	for _, q := range grade.Questions {
		b.WriteString(`<h3 style="margin-bottom:4px">` + html.EscapeString(q.Question+" — "+q.Title) + `</h3>`)
		b.WriteString(`<ul style="margin-top:4px">`)
		for _, it := range q.Items {
			marca := "✗"
			color := "#b91c1c"
			if it.Correct {
				marca = "✓"
				color = "#15803d"
			}
			b.WriteString(fmt.Sprintf(
				`<li><span style="color:%s;font-weight:bold">%s</span> %s — %s <em>(%.2f/%.2f)</em></li>`,
				color, marca, html.EscapeString(it.Label), html.EscapeString(it.Selected), it.Points, it.Max,
			))
		}
		b.WriteString(`</ul>`)
	}

	b.WriteString(`<p style="color:#6b7280;font-size:13px;margin-top:24px">Correo automático — CBTIC.</p>`)
	b.WriteString(`</div>`)

	return b.String()
}

func statusLabel(status string) string {
	switch status {
	case StatusCompleted:
		return "Completado"
	case StatusManual:
		return "Terminado por el estudiante"
	case StatusTimeout:
		return "Tiempo agotado"
	case StatusFraud:
		return "Anulado por fraude"
	default:
		return status
	}
}
