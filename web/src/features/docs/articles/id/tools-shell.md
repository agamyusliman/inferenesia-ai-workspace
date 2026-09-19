---
title: Tool & shell
category: tools
summary: Terminal multi-tab dan split, snippet, overview browser tools, shortcut keyboard
keywords: terminal shell tab split snippet browser keyboard shortcut tools
order: 70
---

# Tool & shell

Inferenesia menggabungkan editor + chat dengan **terminal** terintegrasi dan tool **browser** opsional yang bisa dijalankan agent. Panduan ini untuk pemakaian harian.

## Dasar terminal

Buka panel **Terminal** dari chrome layout (bawah / area dock, tergantung layout).

| Fitur | Hasil |
|-------|-------|
| Multi-tab | Beberapa shell di satu workspace |
| Split | Terminal side-by-side |
| Default cwd | Root workspace saat folder aktif |
| Dari explorer | **Open in Integrated Terminal** di folder |

Tiap tab = session shell sendiri. Tutup tab = session itu berakhir; tab lain tetap jalan.

## Multi-tab dan split

1. Buka Terminal.
2. **New tab** untuk shell kedua (test di satu, server di lain).
3. **Split** kalau mau dua pane kelihatan bareng (log + command).
4. Klik pane dulu sebelum mengetik supaya fokus benar.

Tips:

- Server long-running di tab khusus supaya tidak ikut terbunuh.
- Konsisten: kiri = app, kanan = test, biar muscle memory nempel.

## Snippet

**Snippet** global menyimpan command pendek yang sering dijalankan:

- `npm run test:unit`
- `git status -sb`
- Baris build khas project

Buat/sisipkan snippet dari chrome terminal bila tersedia. Snippet untuk kamu; agent tetap punya tool shell sendiri lewat chat.

## Browser tools (overview)

Agent bisa memakai tool browser bila setup mengizinkan (buka page, inspect, screenshot, check). Dari sisi kamu:

1. Minta di chat: “Buka app lokal dan cek form login.”
2. Lihat blok tool di reply untuk apa yang dijalankan.
3. Jangan paste cookie/session token seenaknya; hasil disanitasi sebisanya, tetap hati-hati.

Tidak perlu IDE browser terpisah untuk kebanyakan task. Kalau browser manusia lebih gampang, pakai browser biasa + lampirkan screenshot ke chat.

## Shell agent vs terminal kamu

| Surface | Siapa yang mengemudi | Cocok untuk |
|---------|----------------------|-------------|
| Terminal terintegrasi | Kamu | Server, CLI interaktif, pantau log |
| Tool shell agent | Agent (dari chat) | Check skrip, command pendek, install yang kamu setujui secara prinsip |

Command destruktif (`rm -rf`, migrate massal): lebih aman kamu jalankan sendiri di terminal setelah baca plan.

## Shortcut keyboard (umum)

Binding bisa sedikit beda per OS/build. Pola yang biasa:

| Aksi | Shortcut (tipikal) |
|------|--------------------|
| Kirim pesan chat | **Enter** |
| Baris baru di composer | **Shift+Enter** |
| Stop generation | Tombol **Stop** atau **Esc** (saat chat fokus) |
| Command palette | **Cmd/Ctrl+Shift+P** (bila ada) |
| Save file | **Cmd/Ctrl+S** |
| Fokus terminal | Klik terminal / shortcut layout bila dikonfigurasi |
| Toggle chat | Kontrol **Chat** di header |

Shortcut tidak bereaksi? Klik panel target dulu supaya fokus benar.

## Tips

- Start dev server di tab terminal, minta agent edit code sambil kamu pantau log.
- Prefer script project (`npm test`) daripada one-liner yang diarang agent, kalau script sudah ada.
- Setelah command agent gagal, paste error atau biarkan ia baca output di follow-up.
- Secrets di env yang di-load app, bukan hard-code di teks snippet yang mungkin dishare.

## Kesalahan umum

| Salah | Lebih baik |
|-------|------------|
| Deploy prod cuma lewat chat agent | Terminal sendiri + review |
| Satu terminal untuk semua | Split tab: server / test / misc |
| Abaikan command tool yang gagal di chat | Expand blok tool, perbaiki root error |

## Lanjut

- [Git](git) — commit setelah perubahan terverifikasi di terminal
- [Tool agent](agent-tools) — skills, MCP, task paralel
- [Diagram & canvas](diagrams-canvas) — tool visual
