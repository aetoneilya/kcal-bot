/**
 * Phone photos are 3–12 MB; the model needs about a megapixel. Downscaling in
 * the browser makes the upload ~200 KB and the model call cheaper, and
 * createImageBitmap applies EXIF orientation so the model sees it upright.
 */
export const MAX_SIDE = 1280;

export function fitWithin(width: number, height: number, max = MAX_SIDE): { width: number; height: number } {
  const scale = Math.min(1, max / Math.max(width, height));
  return { width: Math.round(width * scale), height: Math.round(height * scale) };
}

export async function downscale(file: Blob): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const size = fitWithin(bitmap.width, bitmap.height);
  const canvas = document.createElement("canvas");
  canvas.width = size.width;
  canvas.height = size.height;
  canvas.getContext("2d")!.drawImage(bitmap, 0, 0, size.width, size.height);
  bitmap.close();
  return new Promise((resolve, reject) =>
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error("toBlob failed"))), "image/jpeg", 0.85),
  );
}

/** The diary's preview of a meal: small enough (~20 KB) to keep for every meal. */
export const THUMB_SIDE = 320;
/** The meal's own screen shows it big: 1080 px is sharp on a phone, ~100 KB. */
export const LARGE_SIDE = 1080;

export const thumbnail = (file: Blob) => resize(file, THUMB_SIDE, 0.72);
export const largePhoto = (file: Blob) => resize(file, LARGE_SIDE, 0.8);

async function resize(file: Blob, side: number, quality: number): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const size = fitWithin(bitmap.width, bitmap.height, side);
  const canvas = document.createElement("canvas");
  canvas.width = size.width;
  canvas.height = size.height;
  canvas.getContext("2d")!.drawImage(bitmap, 0, 0, size.width, size.height);
  bitmap.close();
  return new Promise((resolve, reject) =>
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error("toBlob failed"))), "image/jpeg", quality),
  );
}

/** Base64 without the data: prefix, as JSON carries bytes. */
export async function base64(blob: Blob): Promise<string> {
  const url: string = await new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
  return url.slice(url.indexOf(",") + 1);
}
