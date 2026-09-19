---
title: Diagram & Playground
category: diagrams
summary: Mermaid, playground HTML, Image Studio (Library atau sesi), edit AI patch, Excalidraw
keywords: diagram mermaid excalidraw playground library image studio sesi generate patch
order: 80
---

# Diagram & Playground

Inferenesia menjaga kerja visual di samping coding: **Mermaid** di chat dan gallery **Playground**, **playground HTML**, **Image Studio**, serta file **Excalidraw**.

## Playground (hub)

Buka item **Playground** di activity rail.

| Area | Yang didapat |
|------|----------------|
| **List** | Item HTML, Diagram, dan **Image** dari **sesi aktif** dan **Library** |
| **Filter** | Semua / HTML / Diagram / Image / (jenis lain menyusul) |
| **Buat** | Pilih jenis → wizard: **judul**, **Tanpa sesi (Library)** (default), atau **sesi chat** |
| **Context menu** | Buka · Ubah nama · Buka sesi (jika bukan Library) · Hapus |
| **Detail HTML** | Frame device, Preview/Code, iframe, bar instruksi ephemeral |
| **Detail diagram** | Editor Mermaid penuh, chip DSL, bar instruksi ephemeral |
| **Image Studio** | Generate / edit gambar: size picker, ref, strip history |

Playground **Library** memakai scope `global`. Membuat dengan **Tanpa sesi (Library)** menjaga prompt generate/edit **di luar riwayat chat sesi**. Memilih sesi mengikat playground ke thread itu (mengirim `workspace_id`).

Waktu di list = **update konten playground terakhir**, bukan chat terakhir di sesi.

Saat Playground terbuka, footer menampilkan `playground: Library` atau `playground: <sesi>`. Status bar shell **tidak** menampilkan branch git, dirty count, atau aksi SCM — gunakan panel **Git**.

### Frame perangkat (HTML)

Preview men-scale **seluruh chrome** (viewport + bezel). Konten mobile tetap di dalam mockup. Perangkat: Mobile 390×844, Tablet 768×1024, Laptop 1280×800, Desktop 1440×900.

## Image Studio

Jenis siap: **Image**.

| Kontrol | Peran |
|---------|--------|
| **Ukuran** (select) | Modal: Landscape / Portrait / Square / **Kertas** (A4, A3, Letter, Legal, Tabloid). Label tombol mis. `Square - 1:1`, `Paper - A4` |
| **Temperature / Thinking / Output** | Image+teks vs image saja; sampling; effort reasoning |
| **Pakai sbg ref** | Checkbox: kirim gambar yang **sedang di-preview** di history sebagai referensi generate berikutnya |
| **History** (rail kiri) | Klik = preview versi itu; **double-klik** versi lama = restore jadi current; thumb 4:3 |
| **Generate** | Model dari **ModelPicker** (sama dengan chat). Field prompt dikosongkan setelah generate sukses |

**Image Studio Library** mengirim `ephemeral: true` dan **tanpa** `workspace_id`, jadi turn **tidak** ditulis ke sesi aktif (mis. Donor Darah). Image Studio yang terikat sesi mengirim id sesi tersebut.

Klik kanan gambar hasil: **Preview layar penuh**, **Unduh**, **Salin prompt user**, **Salin prompt agent** (prompt yang benar-benar dikirim untuk generate itu).

## Mermaid di chat

Fence Mermaid di balasan asisten bisa dirender di bubble. Fence selesai dari sesi juga bisa **diindeks** ke Playground sebagai Diagram.

## Edit AI pada HTML / diagram (gallery)

Bar instruksi di bawah bersifat **ephemeral** (bukan riwayat chat sesi). Permintaan biasa memakai **patch** SEARCH/REPLACE, bukan rewrite penuh, kecuali kamu minta redesign.

## File Excalidraw

File `.excalidraw` terbuka di canvas visual dengan view **Source** bila perlu. **Save** saat dirty.

## Mermaid vs HTML vs Image Studio vs Excalidraw

| Prefer Mermaid | Prefer playground HTML | Prefer Image Studio | Prefer Excalidraw |
|----------------|------------------------|---------------------|-------------------|
| Flow/sequence di chat/gallery | Mock UI live | Still generate / edit ref | Whiteboard bebas |
| Export SVG/PNG | Frame device + code | Ukuran/kertas, history, ref | File di repo |
| Masuk Playground | Scope Library atau sesi | Scope Library atau sesi | File repo |

## Tips

- Instruksi edit yang spesifik membantu mode patch.
- Setelah generate Image Studio, pastikan footer masih **playground: Library** jika kamu pilih tanpa sesi.
- Jangan taruh secret di label diagram, copy HTML, atau prompt gambar.
- Pakai **Ubah nama** di context menu jika judul otomatis salah.

## Kesalahan umum

| Salah | Lebih baik |
|-------|------------|
| Mengira chat sesi akan mengedit playground Library | Pakai bar playground / Image Studio, atau buka item Library |
| Hub lama masih jalan setelah perbaikan image-gen | Rebuild/restart `inferenesia-desktop` |
| Mengira waktu list = chat terakhir | List menampilkan `updatedAt` konten playground |
| **Pakai sbg ref** tanpa cek preview history | Preview dulu versi yang diinginkan, lalu centang **Pakai sbg ref** |

## Lanjut

- [Studi kasus](case-studies)
- [Chat & model](chat-models) — picker model dipakai Image Studio
- [Git](git) — commit diagram dan aset bersama kode
