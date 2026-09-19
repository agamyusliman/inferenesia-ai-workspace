---
title: Plan Mode & Mission
category: plan
summary: Kapan Plan Mode, approve/reject plan, dan Mission dengan evidence dalam bahasa sederhana
keywords: plan mode mission approve reject evidence todos goal safe write
order: 40
---

# Plan Mode & Mission

Dua rel pengaman biar agent tidak “rewrite setengah repo tanpa sengaja.”

- **Plan Mode** — agent boleh baca dan usul, tapi write file diblok sampai kamu keluar Plan Mode / approve alur plan di build kamu.
- **Mission** — goal dengan checkpoint + evidence (test, log, check) supaya “selesai” bukan cuma omongan model.

Bukan fitur yang sama. Plan Mode = kunci write + stance merencana. Mission = eksekusi yang digate evidence.

## Plan Mode: kapan dipakai

Nyalakan **Plan Mode** kalau:

- Belum yakin file mana yang harus berubah.
- Perubahan menyentuh auth, payment, migration, atau API bersama.
- Mau daftar langkah dulu sebelum edit.
- Explore codebase baru, analisis tanpa write.

Matikan untuk edit kecil yang sudah jelas (“rename label ini”, “perbaiki typo”).

## Apa yang terjadi di Plan Mode

| Perilaku | Di Plan Mode |
|----------|--------------|
| Baca file, search, jelaskan | Boleh |
| Usulkan plan / todos | Boleh |
| Tool mutasi (write, banyak shell edit, dll.) | Ditolak dengan pesan gaya “plan mode: writes denied” |
| Edit manual di editor | Tetap milikmu; Plan Mode menarget **agent** |

Kalau agent coba write saat Plan Mode on, write tidak mendarat. Itu fitur.

## Approve atau reject plan

Alur UI bisa sedikit beda per build:

1. Aktifkan **Plan Mode** dari kontrol chat / plan.
2. Minta plan: “Outline cara menambahkan rate limiting di route login. Jangan edit dulu.”
3. Baca plan dan todos.
4. **Approve** kalau mau lanjut eksekusi (keluar Plan Mode atau ikuti alur approve di UI).
5. **Reject** / revisi: kasih feedback (“skip ubah database, middleware saja”) lalu minta plan baru.

Prompt plan yang bagus:

- Constraint: “Tanpa dependency baru.”
- Out of scope: “Jangan sentuh package billing.”
- Minta daftar file: “List setiap path yang akan diedit.”

## Mission: evidence dengan kata sederhana

**Mission** = goal besar dipecah jadi langkah di mana “done” lebih dari chat yang percaya diri. Evidence bisa:

- Output test yang lulus
- Command yang exit bersih
- Screenshot / cuplikan log yang dikumpulkan agent
- Item checklist yang kamu konfirmasi di UI

Anggap Mission: **goal → langkah → bukti → langkah berikutnya**, bukan ngobrol bebas sampai model bilang selesai.

### Kapan Mission membantu

| Cocok | Kurang cocok |
|-------|--------------|
| Refactor multi-step + test | Ubah copy satu baris |
| “Bikin CI hijau” | Opini desain murni |
| Fitur lintas file + verifikasi | Brainstorm saja (pakai Session + Plan) |

### Menjalankan mission

1. Nyatakan goal + kriteria sukses (“`npm test` lulus”, “login 401 tanpa cookie”).
2. Biarkan agent usulkan langkah + evidence per langkah.
3. Jalankan/izinkan check; jangan skip bukti di langkah berisiko.
4. Kalau evidence gagal, perbaiki atau replan sebelum mission ditandai complete.

## Plan vs Mission vs chat biasa

| Mode | Write | Paling cocok |
|------|-------|--------------|
| Chat biasa | Boleh (dengan safety undo) | Task kecil, scope jelas |
| **Plan Mode** | Diblok untuk agent sampai keluar / approve | Desain dulu, risiko tinggi |
| **Mission** | Boleh di alur mission | Kerja multi-step yang butuh bukti |

Boleh plan dulu (Plan Mode), lalu Mission setelah arah jelas.

## Tips

- Plan pendek menang lawan novel 40 langkah. Target 3–7 langkah konkret.
- Selalu sebut check sukses di pesan pertama mission.
- Setelah approve, pantau edit pertama; **Revert agent** tetap ada.
- Agent masih coba write di Plan Mode? Prompt kamu mungkin mendorong “langsung implement”. Tulis ulang: “Tetap plan only.”

## Kesalahan umum

| Salah | Lebih baik |
|-------|------------|
| Plan Mode on selamanya untuk edit kecil | Matikan untuk perubahan aman kecil |
| Approve tanpa baca daftar file | Skim path dulu |
| Mission tanpa test/command | Definisikan evidence di awal |
| Mission disamakan Plan Mode | Plan = no write; Mission = eksekusi + bukti |

## Lanjut

- [Undo / Revert agent](undo-revert) — pulih kalau eksekusi meleset
- [Tool agent](agent-tools) — skills, MCP, explore paralel
- [Studi kasus](case-studies) — walkthrough edit aman
