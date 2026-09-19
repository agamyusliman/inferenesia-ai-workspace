---
title: Studi kasus
category: cases
summary: Dua walkthrough praktis: edit aman pertama, dan diagram/canvas bersama agent
keywords: case study walkthrough safe edit diagram canvas tutorial example
order: 110
---

# Studi kasus

Dua walkthrough pendek yang bisa ditiru. Asumsi: app desktop, **workspace** folder, dan model yang sudah jalan di **Settings**.

---

## Kasus 1: Edit aman pertama

**Goal:** Ubah file nyata lewat agent, review, dan tetap punya jalan keluar mudah.

### Setup

1. **Workspaces** → buka folder project.
2. Pastikan status Git bersih, atau kamu tahu apa yang sudah dirty.
3. Opsional: buat branch untuk eksperimen.
4. Buka **Chat**, pilih model yang kamu percaya untuk edit kecil.

### Langkah

1. **Mention file**  
   Di composer, ketik `@` dan pilih file yang mau diubah (contoh: `README.md` atau file string UI kecil).

2. **Minta perubahan kecil yang bisa dicek**  
   Contoh prompt:

   > Di `@README.md`, tambahkan bullet “Requirements” singkat bahwa kita butuh Node 20+. Jangan edit file lain.

3. **Pantau turn**  
   Expand blok tool kalau mau lihat edit. Tunggu Generating selesai.

4. **Review**  
   - Buka file di editor.  
   - Atau buka **Git** dan baca diff.  
   - Atau buka **Agent changes** di toolbar safety.

5. **Putuskan**

   | Hasil | Aksi |
   |-------|------|
   | Oke | Simpan; stage + commit saat siap |
   | Hampir oke | Rapikan manual |
   | Salah | **Revert agent** (atau **Undo**) sebelum edit numpuk |

6. **Commit (opsional tapi sehat)**  

   > Contoh message: “docs: note Node 20 requirement”

### Kenapa ini “aman”

- Scope satu file, satu perubahan jelas.
- Review sebelum commit.
- **Revert agent** mengembalikan dirty pre-agent, bukan cuma git HEAD.

### Variasi

- Nyalakan **Plan Mode** dulu, minta plan; approve hanya bila daftar file satu path.
- **Stop** kalau agent mulai sentuh file tak terkait, lalu tulis ulang dengan scope lebih ketat.

---

## Kasus 2: Diagram / canvas

**Goal:** Hasilkan diagram yang bisa dishare lewat Mermaid atau canvas Excalidraw.

### Jalur A — Mermaid di doc

1. Buka/buat `docs/overview.md` di workspace.
2. Prompt:

   > Tambahkan flowchart Mermaid di `docs/overview.md` yang menunjukkan: User → Inferenesia desktop → Agent → Project files. Maksimal lima node.

3. Buka **Preview** markdown dan cek diagram.
4. Perbaiki label di editor atau minta: “Rename node Agent menjadi Chat agent.”
5. Save, lalu commit kalau tim harus melihatnya.

### Jalur B — Canvas Excalidraw

1. Minta:

   > Buat `docs/diagrams/request-flow.excalidraw` dengan box Client, API, dan Database, plus arrow kiri ke kanan.

2. Buka file `.excalidraw` baru.
3. Pakai **Canvas** untuk rapikan layout dan label.
4. Opsional AI edit:

   > Di canvas yang terbuka, tambah box Cache antara API dan Database.

5. **Save** saat indikator dirty muncul.
6. Review di **Git**, commit dengan nama jelas.

### Checklist review

| Cek | OK? |
|-----|-----|
| Nama file masuk akal di repo | |
| Tidak ada secret di label | |
| Diagram cukup cocok sistem nyata untuk diskusi | |
| File tersimpan dan, bila perlu, ter-commit | |

### Kalau AI kelebihan gambar

- Hapus shape ekstra manual di canvas.
- Atau **Revert agent** kalau file utuh lebih baik sebelum turn.
- Minta edit lebih kecil: “Tambah satu arrow saja, jangan acak-acak layout.”

---

## Kebiasaan yang ikut terbawa

1. **Scope kecil** di prompt pertama.  
2. **Review** di editor, Git, atau Agent changes.  
3. **Revert agent** cepat kalau salah; **commit** kalau benar.  
4. Prefer **Plan Mode** saat kamu belum bisa sebut daftar file.  
5. Simpan diagram dekat docs yang sudah dibaca tim.

## Lanjut

- [Undo / Revert agent](undo-revert)
- [Plan Mode & Mission](plan-mission)
- [Diagram & canvas](diagrams-canvas)
- [Chat & model](chat-models)
