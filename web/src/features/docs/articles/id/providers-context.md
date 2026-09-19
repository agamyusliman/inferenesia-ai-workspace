---
title: Provider & context
category: providers
summary: Hubungkan Inferenesia API atau BYOK, Token Savers on/off, jaga context chat tetap ramping
keywords: provider api key byok gateway model token saver context settings
order: 100
---

# Provider & context

Chat butuh **provider** (tempat model) dan **context** yang masih muat di tiap turn. Atur di **Settings**, lalu pilih model di chat.

## Dua cara hubungkan model

| Jalur | Apa itu | Key |
|-------|---------|-----|
| **Inferenesia API** | Gateway hosted untuk model | `INFERENESIA_API_KEY` (plus base URL kalau dikustom) |
| **BYOK** | Bawa key sendiri; traffic **langsung** ke provider yang kamu set | API key provider, disimpan lokal |

Boleh satu atau keduanya. BYOK **tidak** mengirim key provider lewat gateway Inferenesia API; ia memanggil base URL yang kamu set.

### Inferenesia API (gateway)

1. Buka **Settings → Providers** (atau setara).
2. Set base URL Inferenesia API bila perlu (default endpoint publik install kamu).
3. Paste **API key**.
4. Save, lalu buka **model picker** di chat dan pilih model.

Env (opsional, CLI / setup lanjutan):

```bash
export INFERENESIA_BASE_URL="https://inferenesia.cloud/v1"
export INFERENESIA_API_KEY="your-key"
# model default opsional
export INFERENESIA_MODEL="..."
```

Config home default `~/.inferenesia` (override: `INFERENESIA_HOME`).

### BYOK (key milikmu)

1. Di **Settings**, tambah profil provider (nama, base URL, API key, model sesuai kebutuhan).
2. Save. Key tetap di mesin kamu.
3. Pilih model profil itu di model picker chat.

Jangan commit key. `.env` tetap lokal. Key bocor? Rotate di sisi provider.

## Model picker

Setelah provider siap:

1. Buka chat.
2. Pakai **model picker** sticky.
3. Pilih model untuk turn berikutnya.

List kosong? Cek key, network, dan profil masih enabled.

## Token Savers

**Token Savers** = helper opsional yang mengurangi seberapa banyak teks dipadatkan ke prompt (output tool lebih pendek, bantuan phrasing lebih ketat, dll.). Default **off**.

| Perilaku | Detail |
|----------|--------|
| Default | Off |
| Helper hilang | Pass-through; chat tetap harus jalan |
| Kapan on | Session panjang, repo besar, tekanan cost/limit |
| Kapan off | Mau detail mentah maksimal di hasil tool |

Toggle di **Settings**. Nyalakan satu, coba task nyata, bandingkan kualitas vs panjang. Jawaban terasa “kurang info”? Matikan lagi.

## Menjaga context tetap ramping

Model cuma “lihat” window terbatas. Kamu bantu dengan:

| Kebiasaan | Kenapa membantu |
|-----------|-----------------|
| Satu goal per pesan | Lebih sedikit noise, tool lebih jelas |
| `@file` ke target nyata | Hindari menjejali file tak terkait |
| Plan Mode untuk desain besar | Lebih sedikit thrash write |
| Session baru untuk topik baru | Turn kuno tidak memenuhi window |
| Jangan paste log utuh | Paste potongan yang gagal |
| Commit / ringkas thread panjang | Start segar dengan brief pendek |

Agent “lupa” instruksi awal? Ulangi constraint di pesan terbaru. Guidance terbaru yang menang di praktik.

## Context vs Multi Brain

- **History chat** — pesan + hasil tool session ini.
- **Catatan Multi Brain** — catatan project opsional yang dibaca agent secara terbatas (lihat [Tool agent](agent-tools)).

Multi Brain untuk fakta project yang tahan lama. Chat untuk task di depan mata.

## Tips

- Model cepat/murah untuk refactor yang bisa di-review cepat; model lebih kuat untuk desain susah.
- Satu profil BYOK per vendor biar key rapi.
- Setelah ganti key, kirim prompt “ping” kecil sebelum job besar.
- CLI share config home yang sama dengan desktop bila `INFERENESIA_HOME` cocok.

## Troubleshooting

| Gejala | Coba |
|--------|------|
| Error auth | Paste ulang API key; cek base URL |
| Model kosong | Profil disabled atau network diblok |
| Jawaban dangkal / terpotong | Matikan Token Savers; kurangi attachment tak relevan |
| Fakta project salah | Pastikan workspace; tambah `@file` atau catatan Multi Brain |

## Lanjut

- [Chat & model](chat-models)
- [Tool agent](agent-tools)
- [Workspace & session](workspaces-sessions)
