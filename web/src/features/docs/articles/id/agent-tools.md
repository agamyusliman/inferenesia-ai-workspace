---
title: Tool agent
category: agent
summary: Skills, MCP tools, task explore paralel, dan catatan Multi Brain yang bisa dibaca agent
keywords: agent skills mcp task explore multi brain memory tools parallel
order: 90
---

# Tool agent

Selain chat polos, agent bisa memakai **tool**: baca/edit file, jalankan shell, load **skills**, bicara ke server **MCP**, explore paralel lewat **task**, dan baca **catatan** project (Multi Brain). Kamu mengarahkan lewat prompt bagus + Settings.

## Yang terlihat di chat

Saat agent pakai tool, reply menampilkan **blok tool** (kadang **task**). Expand untuk lihat apa yang dijalankan. Tidak perlu hafal nama tool; baca label dan hasilnya.

## Skills

**Skills** = paket instruksi yang bisa di-load on demand (playbook fokus untuk satu job).

Alur tipikal:

1. Kamu minta sesuatu yang cocok skill (“pakai skill checklist release”).
2. Agent load konten skill.
3. Ia mengikuti panduan itu untuk sisa turn/task.

Tips:

- Skill project/user muncul bila dikonfigurasi di install kamu.
- “Ikuti skill X” lebih hemat daripada paste checklist panjang tiap kali.
- Skill hilang? Agent seharusnya lanjut tanpa crash; install/enable di environment kalau memang butuh.

## MCP tools

**MCP** (Model Context Protocol) menghubungkan server tool eksternal: issue tracker, host docs, API internal, dll.

Dari sisi kamu:

1. Konfigurasi server MCP di **Settings** / config (sesuai dukungan build).
2. Minta agent memakainya dengan bahasa natural (“list bug open untuk repo ini”).
3. Review output tool di chat sebelum percaya side effect.

Keamanan:

- Hanya sambungkan MCP yang kamu percaya.
- Prefer tool read-only saat explore.
- Jangan commit secret production di config MCP.

## Task explore paralel

Untuk pertanyaan lebar (“di mana auth di-handle?”), agent bisa memecah kerja jadi **explore** paralel, lalu menggabungkan hasil.

Yang perlu kamu tahu:

| Ide | Arti untukmu |
|-----|----------------|
| Explore paralel | Beberapa search fokus sekaligus |
| Batas depth | Nested “task di dalam task” dibatasi biar run tidak meledak |
| Synthesize | Prefer satu jawaban gabungan setelah explore, bukan search ulang area yang sama |

Bisa dorong: “Explore `src/auth` dan `web/auth` paralel, lalu ringkas saja.”

Blok task berisik? Minta sintesis: “Kasih tiga file yang penting dan kenapa.”

## Multi Brain (catatan project)

Kalau project punya folder **Multi Brain** (catatan markdown navigable untuk agent), Inferenesia bisa membiarkan agent membaca **potongan terbatas**:

- Index master singkat dulu
- Satu–dua list topik yang cocok
- Catatan dalam hanya bila index menunjuk ke situ

Anggap ini **catatan yang bisa dibaca agent**, bukan history chat kedua, dan bukan dump semua file ke prompt.

### Hygiene Multi Brain

| Lakukan | Jangan |
|---------|--------|
| Index topik tetap pendek | Paste log utuh ke index |
| Arahkan ke file detail untuk keputusan | Commit secret ke catatan |
| Update setelah kerja bermakna | Harap agent mengarang history yang tidak pernah ditulis |

Multi Brain biasanya **lokal** (sering gitignored). Kebenaran product untuk tim tetap di `docs/` biasa.

## Tema tool bawaan (sudut pandang user)

| Tema | Contoh yang mungkin dilakukan agent |
|------|-------------------------------------|
| Files | Baca, search, edit di dalam workspace |
| Shell | Jalankan command project |
| Git | Status, diff, stage, commit sesuai praktik yang kamu izinkan |
| Browser | Buka page, check, screenshot bila diaktifkan |
| Diagrams | Buat/update Mermaid / canvas |
| Memory | Baca catatan Multi Brain bila ada |

Write file tetap di dalam sandbox root workspace aktif.

## Tips

- Sebut kelas tool di prompt kalau penting: “Pakai browser tools untuk verifikasi form.”
- Codebase besar: minta explore paralel + satu ringkasan.
- Setelah MCP write (ticket, comment), verifikasi di sistem eksternal.
- Plan Mode tetap memblok tool mutasi sampai kamu keluar/approve.

## Kesalahan umum

| Salah | Lebih baik |
|-------|------------|
| Harap MCP tanpa konfigurasi | Tambah server di Settings / config dulu |
| Paste secret ke file skill di git | Secret hanya di env lokal |
| Search ulang yang sama lima kali | Minta synthesize hasil task yang ada |

## Lanjut

- [Plan Mode & Mission](plan-mission)
- [Provider & context](providers-context)
- [Tool & shell](tools-shell)
