package mail

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/queue"
)

type shipmentMailTemplateData struct {
	Subject      string
	Message      string
	DownloadURL  string
	ExpiresAtJST string
}

var shipmentHTMLTemplate = template.Must(template.New("shipment_html").Parse(`<!doctype html>
<html>
  <body>
    <p>Secure Send からファイル共有のお知らせです。</p>
    <p><strong>件名:</strong> {{.Subject}}</p>
    {{if .Message}}<p><strong>メッセージ:</strong><br>{{.Message}}</p>{{end}}
    <p><a href="{{.DownloadURL}}">ダウンロードページを開く</a></p>
    {{if .ExpiresAtJST}}<p>有効期限: {{.ExpiresAtJST}}</p>{{end}}
    <p>※このメールに心当たりがない場合は破棄してください。</p>
  </body>
</html>`))

func BuildShipmentNotification(frontendURL string, msg queue.MailNotification) (Body, error) {
	base := strings.TrimRight(frontendURL, "/")
	downloadURL := fmt.Sprintf("%s/r/%s", base, msg.Token)
	expires := ""
	if msg.ExpiresAt != nil {
		expires = msg.ExpiresAt.UTC().Format(time.RFC3339)
	}
	content := ""
	if msg.Message != nil {
		content = *msg.Message
	}
	data := shipmentMailTemplateData{
		Subject:      msg.Subject,
		Message:      content,
		DownloadURL:  downloadURL,
		ExpiresAtJST: expires,
	}

	var htmlBuilder strings.Builder
	if err := shipmentHTMLTemplate.Execute(&htmlBuilder, data); err != nil {
		return Body{}, fmt.Errorf("execute html template: %w", err)
	}

	text := fmt.Sprintf("Secure Send からファイル共有のお知らせです。\n\n件名: %s\n", data.Subject)
	if data.Message != "" {
		text += fmt.Sprintf("メッセージ:\n%s\n\n", data.Message)
	}
	text += fmt.Sprintf("ダウンロードURL: %s\n", data.DownloadURL)
	if data.ExpiresAtJST != "" {
		text += fmt.Sprintf("有効期限: %s\n", data.ExpiresAtJST)
	}
	text += "\n※このメールに心当たりがない場合は破棄してください。\n"

	return Body{Text: text, HTML: htmlBuilder.String()}, nil
}
