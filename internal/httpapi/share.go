package httpapi

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

func (r *router) shareYurubo(w http.ResponseWriter, req *http.Request) {
	id, msg := cleanUUID(req.PathValue("id"), "yurubo id")
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	scheme := "app.ohey.com"
	if strings.EqualFold(r.deps.Config.Environment, "dev") || strings.EqualFold(r.deps.Config.Environment, "development") {
		scheme = "app.ohey.com.dev"
	}
	appURL := fmt.Sprintf("%s://yurubos/%s", scheme, id)
	// Store IDs are not guaranteed before public release, so fall back to the
	// platform search pages where users can install Ohey when it is available.
	appStoreURL := "https://apps.apple.com/search?term=Ohey"
	playStoreURL := "https://play.google.com/store/search?q=Ohey&c=apps"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Oheyのゆるぼ</title>
  <meta name="description" content="Oheyでこのゆるぼに参加しよう。">
  <style>
    body { margin: 0; min-height: 100vh; display: grid; place-items: center; font-family: -apple-system, BlinkMacSystemFont, "Helvetica Neue", sans-serif; background: #071038; color: white; }
    main { width: min(88vw, 420px); padding: 28px; border-radius: 28px; background: #0A1521; box-shadow: 0 18px 48px rgba(0,0,0,.36); }
    h1 { margin: 0 0 10px; font-size: 28px; }
    p { color: rgba(255,255,255,.72); font-weight: 700; line-height: 1.55; }
    a { display: block; margin-top: 14px; padding: 16px 18px; border-radius: 999px; text-align: center; color: #06111D; background: #C08BFF; font-weight: 900; text-decoration: none; }
    .install { background: #9AF21A; }
  </style>
</head>
<body>
  <main>
    <h1>Oheyのゆるぼ</h1>
    <p>アプリを開いて、このゆるぼに参加します。開かない場合はインストールしてからもう一度リンクを開いてください。</p>
    <a href="%s">Oheyで参加する</a>
    <a class="install" id="install" href="%s">Oheyをインストール</a>
  </main>
  <script>
    const appUrl = %q;
    const appStoreUrl = %q;
    const playStoreUrl = %q;
    const isAndroid = /Android/i.test(navigator.userAgent);
    const isIOS = /iPhone|iPad|iPod/i.test(navigator.userAgent);
    const installUrl = isAndroid ? playStoreUrl : appStoreUrl;
    document.getElementById('install').href = installUrl;
    window.location.href = appUrl;
    setTimeout(() => { window.location.href = installUrl; }, 1400);
  </script>
</body>
</html>`, html.EscapeString(appURL), html.EscapeString(appStoreURL), appURL, appStoreURL, playStoreURL)
}
