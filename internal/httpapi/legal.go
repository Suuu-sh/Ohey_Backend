package httpapi

import (
	"net/http"
	"time"
)

const legalTermsText = `Ohey 利用規約

この利用規約は、Ohey の利用条件を定めるものです。

1. Ohey は、友だちとの予定・気分・やりたいことの共有を支援するサービスです。
2. 利用者は、法令または公序良俗に反する投稿、第三者を害する行為、不正アクセス、迷惑行為を行ってはいけません。
3. 投稿内容やアカウント情報は、サービス提供、保守、安全性向上のために必要な範囲で取り扱います。
4. 運営者は、必要に応じて機能の変更、利用制限、投稿削除、アカウント停止を行うことがあります。
5. サービスは現状有姿で提供され、運営者は法令上認められる範囲で、利用により生じた損害について責任を負いません。
6. お問い合わせはアプリ内に記載のサポート窓口までご連絡ください。

制定日: 2026-06-09
`

const legalPrivacyText = `Ohey プライバシーポリシー

Ohey は、利用者のプライバシーを尊重し、以下の方針で情報を取り扱います。

1. 取得する情報: メールアドレス、ユーザーID、表示名、プロフィール、友だち関係、予定・ステータス・通知に関する情報、端末の push token など。
2. 利用目的: アカウント管理、本人確認、サービス提供、通知配信、安全性の確保、不具合調査、改善のため。
3. 第三者提供: 法令に基づく場合、利用者の同意がある場合、またはサービス提供に必要な外部事業者へ委託する場合を除き、第三者へ提供しません。
4. 外部サービス: 認証、データベース、通知、決済、分析などに外部サービスを利用することがあります。
5. 保管期間: 利用目的に必要な範囲で保管し、不要になった情報は合理的な方法で削除または匿名化します。
6. 開示・削除等: ご本人からの請求には、法令に従い合理的な範囲で対応します。
7. お問い合わせはアプリ内に記載のサポート窓口までご連絡ください。

制定日: 2026-06-09
`

func (r *router) legalTerms(w http.ResponseWriter, req *http.Request) {
	writeLegalText(w, req, legalTermsText)
}

func (r *router) legalPrivacy(w http.ResponseWriter, req *http.Request) {
	writeLegalText(w, req, legalPrivacyText)
}

func writeLegalText(w http.ResponseWriter, _ *http.Request, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Last-Modified", time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat))
	_, _ = w.Write([]byte(body))
}
