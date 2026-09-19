---
title: Chat & model
category: chat
summary: Kirim pesan, stop, @file, lampiran, model picker, Live Blocks
keywords: chat model stop esc mention attachment live blocks stream prompt picker
order: 20
---

# Chat & model

Chat adalah cara utama kerja dengan agent: jelaskan perubahan, kasih context, pilih model, lalu lihat reply yang stream.

## Layout

| Bagian | Peran |
|--------|-------|
| Daftar pesan | Prompt kamu + reply agent, plus blok tool dan task |
| Composer | Teks, lampiran, @ mention, Send |
| Model picker | Pilih model percakapan tanpa keluar dari chat |
| Chrome Plan / todos | Badge Plan Mode, dialog proposal, sidebar todo saat aktif |
| Indikator Generating | Muncul selama reply masih terbuka |

Tampilkan/sembunyikan chat lewat toggle **Chat** di header. Samakan dengan **workspace** atau **session** aktif biar history cocok.

## Kirim pesan

1. Fokus ke composer.
2. Ketik prompt. **Enter** kirim; **Shift+Enter** baris baru.
3. Opsional: lampiran atau `@file` (di bawah).
4. Send, lalu pantau stream.

Chat kosong menyediakan starter sesuai konteks: eksplorasi/review proyek di workspace, perencanaan/debugging di sesi. Starter hanya mengisi composer kosong lalu memfokuskannya; ubah draft dan tekan Send saat siap. Starter tidak mengirim permintaan atau menimpa draft yang sudah ada.

Saat agent kerja, bisa muncul tool call (baca file, jalankan command, edit, dll.) di dalam jawaban.

## Stop

| Kontrol | Efek |
|---------|------|
| **Stop** | Akhiri stream / turn aktif dari UI |
| **Esc** | Batalkan streaming bila panel chat mengizinkan; juga menutup banyak overlay |

Composer read-only saat stream supaya **Esc** tidak “dimakan” ketikan. Ganti workspace di tengah stream juga membatalkan stream di client; turn tetap terhubung ke workspace tempat dimulainya.

## @file mention

Ketik `@` di composer, pilih path dari project. Mention mengaitkan file itu ke turn supaya agent lebih cenderung baca context yang benar tanpa kamu paste seluruh isi.

Tips:

- Satu–dua file fokus lebih baik daripada dump seluruh folder.
- Mention file yang mau diubah, bukan semua module terkait.
- Folder generate besar biasanya noise; skip kecuali task memang butuh.

## Lampiran (attachment)

Bisa lampirkan image, PDF, dan file teks dari composer. Cocok kalau:

- Bug-nya visual (screenshot).
- Ada log/spec pendek yang belum ada di repo.
- Model perlu lihat dokumen di luar root workspace.

Jaga tetap kecil dan relevan. Binary besar boros context dan memperlambat turn.

## Pilih model

Pakai **model picker** yang sticky di dekat composer:

1. Buka picker.
2. Pilih model dari provider yang terhubung (Inferenesia API dan/atau profil BYOK di **Settings**).
3. Kirim pesan berikutnya dengan model itu.

Pilihan model berlaku untuk turn baru. Kalau model hilang, cek **Settings → Providers** untuk key dan base URL.

## Live Blocks

**Live Blocks** bisa merender preview HTML aman dari sebagian konten assistant di iframe yang terisolasi. Default **off**.

| Setting | Di mana |
|---------|---------|
| Live Blocks on/off | **Settings** (pref workspace atau app, tergantung build) |

Nyalakan hanya kalau mau preview kaya. Matikan kalau cukup markdown polos.

## Web Preview (panel penuh)

Untuk landing page dan server lokal, buka **Preview** di rail aktivitas — **panel utama penuh** (bukan belah chat):

| Mode | Pakai untuk |
|------|-------------|
| **URL** | Live server (`127.0.0.1:8080`, Vite, dll.) |
| **File** | Muat HTML dari sesi atau workspace aktif |
| **HTML** | Edit HTML lalu Apply agar iframe refresh |

Frame perangkat: **Mobile**, **Tablet**, **Laptop**, **Desktop**. Bisa diganti kapan saja; frame menyesuaikan ukuran stage.

Ini terpisah dari **canvas** Excalidraw (diagram) dan Live Blocks di dalam chat.

## Membaca reply

Turn assistant biasanya bisa berisi:

- Teks stream (jawaban utama)
- Blok tool (apa yang dijalankan/diedit)
- Blok task (explore paralel bila dipakai)
- Catatan thinking/meta singkat bila model menyediakannya

Tidak perlu buka semua blok tool. Expand yang relevan saat ada yang aneh.

## Tips

- Satu goal jelas per pesan lebih baik daripada daftar panjang.
- Sebut file/fitur: “Di `src/auth.ts`, perbaiki null check pada `user`.”
- **Stop** lebih awal kalau agent melenceng, lalu tulis ulang prompt.
- Ganti model untuk reasoning berat vs edit cepat/murah bila provider mengizinkan.
- Plan Mode (lihat [Plan Mode & Mission](plan-mission)) lebih aman daripada berharap agent “cuma lihat” sebelum write.

## Troubleshooting

| Gejala | Coba |
|--------|------|
| “Generating” kosong/macet | Stop, cek network + API key di Settings, kirim ulang |
| List model kosong | Set Inferenesia API atau BYOK di **Settings → Providers** |
| Agent mengabaikan file | Tambah `@file` atau sebut path di teks |
| Context project salah | Pastikan **workspace** aktif di rail Workspaces |

## Lanjut

- [Workspace & session](workspaces-sessions) — di mana history disimpan
- [Undo / Revert agent](undo-revert) — kalau turn merusak file
- [Provider & context](providers-context) — key, Token Savers, context tetap ramping
