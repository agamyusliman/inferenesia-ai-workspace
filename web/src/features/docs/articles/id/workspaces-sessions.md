---
title: Workspace & session
category: workspace
summary: Buka dan ganti folder project vs Sessions chat-only, plus apa yang berubah di UI
keywords: workspace session open switch remove explorer git chat terminal folder
order: 10
---

# Workspace & session

Di Inferenesia, **workspace** (folder project) dan **session** (thread chat tanpa project) beda. Kalau dicampur, biasanya chat “hilang” karena kamu buka rail yang salah.

## Peta cepat

| Surface | Apa itu | Yang kamu dapat |
|---------|---------|-----------------|
| **Workspaces** | Folder asli di disk | Explorer, editor, panel Git, terminal, tool file agent, chat untuk project itu |
| **Sessions** | Thread chat tanpa root project | Chat saja (tanpa tree, tanpa panel Git) |

Klik project ≠ ganti tab chat. Masing-masing punya history chat sendiri.

## Buka workspace

1. Activity rail → **Workspaces**.
2. Klik **Open workspace…**
3. Pilih folder recent, klik **Browse folder…**, atau ketik path folder.
4. Inferenesia mendaftarkan folder itu, menjadikannya aktif, lalu load explorer + chat-nya.

Dialog langsung memfokuskan **Browse folder…**. Tab dan Shift+Tab berpindah antar-kontrol di dalam dialog; Esc menutupnya dan mengembalikan fokus ke tombol pembuka.

Path yang sama dibuka dua kali dipakai ulang. Kalau folder pindah/terhapus, pakai **Relink…**.

### Dari CLI

```bash
inferenesia open /path/to/project
```

Ini mendaftarkan workspace yang sama jenisnya dengan desktop.

## Ganti workspace

| Aksi | Efek |
|------|------|
| Klik workspace lain di list | Root aktif berganti; explorer, Git, terminal ikut; history chat pindah ke workspace itu |
| Ganti saat reply masih stream | Stream yang jalan di-stop; reply yang sudah selesai tetap di workspace tempat turn dimulai |

Boleh daftar banyak project dan loncat-loncat tanpa tutup Inferenesia.

## Hapus workspace dari list

Remove cuma unregister dari Inferenesia. File di disk **tidak** ikut terhapus. History chat id itu tidak lagi tampil di rail.

## Apa yang berubah saat folder aktif

| Area | Perilaku |
|------|----------|
| **Explorer** | Tree dari root project (noise umum seperti `node_modules` disembunyikan) |
| **Editor** | Tab buka file di bawah root; Save menulis ke project |
| **Git** | Status, stage, commit, fetch, pull, push di panel **Git** (bukan footer shell) |
| **Chat** | History + tool agent di-scope ke workspace ini |
| **Terminal** | Working directory default = root workspace |
| **Undo / Revert agent** | Stack **per workspace**, tidak digabung lintas project |
| **Status bar** | Product + label konteks (`workspace`, `session: …`, atau `playground: …`) + plan/theme — **tanpa** strip branch git |

Agent tidak bisa write di luar root yang didaftarkan. Edit tetap di project yang kamu buka.

## Sessions (chat-only)

Pakai **Sessions** untuk thread yang tidak terhubung ke repo: brainstorm, desain API, tanya-jawab umum.

1. Activity rail → **Sessions**.
2. Buat atau pilih session.
3. Chat seperti biasa. Tidak ada explorer/Git untuk item ini.

Boleh paste snippet. Untuk edit file beneran, buka **workspace**.

## Tips

- Nama workspace = nama project, bukan nama task. Pakai judul chat atau session untuk thread sementara.
- Satu workspace per git root lebih rapi. Nested repo bisa muncul sebagai chip di Git.
- Config + daftar workspace ada di `~/.inferenesia` (atau `INFERENESIA_HOME`). Itu terpisah dari folder project.
- Chat kosong setelah switch? Cek kamu di **Workspaces** atau **Sessions**. History tidak digabung antar rail.

## Kesalahan umum

| Salah | Lebih baik |
|-------|------------|
| Harap Git di Session | Buka folder workspace |
| Cari chat semalam di project lain | Switch ke workspace yang dipakai dulu |
| Hapus workspace untuk “bersihin disk” | Remove cuma unregister; hapus file di file manager OS kalau memang mau |

## Lanjut

- [Chat & model](chat-models) — kirim prompt, pilih model, stop
- [Undo / Revert agent](undo-revert) — pulihkan file setelah turn agent
- [Git](git) — stage, commit, push dari desktop
