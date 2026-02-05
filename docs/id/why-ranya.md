# Kenapa Ranya

Gunakan halaman ini untuk memutuskan cepat apakah Ranya cocok.

## Pakai Ranya Jika

- Butuh telephony real‑time dengan **barge‑in**.
- Butuh **pipeline deterministik** untuk debug.
- Butuh **tool safety** (konfirmasi, retry, idempotensi).
- Perlu **portabilitas vendor**.

## Pertimbangkan Ulang Jika

- Hanya butuh chatbot sederhana tanpa streaming.
- Tidak butuh tool safety atau observability.
- Tidak akan ganti provider.

## Tradeoff

- Deterministik berarti flow ketat, bukan dinamis.
- Konfigurasi lebih banyak di awal untuk safety di production.

## Jalur Tercepat

- Jalankan contoh HVAC.  
  [Contoh](examples.md)

- Ikuti langkah minimal.  
  [Mulai di Sini](start-here.md)
