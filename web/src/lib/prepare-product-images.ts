// Browser-side resizing reduces both upstream transfers. The option is explicit
// because the optimized file replaces the original in the image library.
export async function prepareProductImage(file: File): Promise<File> {
  if (
    !["image/jpeg", "image/png", "image/webp"].includes(file.type) ||
    typeof createImageBitmap !== "function"
  )
    return file;
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
  } catch {
    return file;
  }
  try {
    const scale = Math.min(1, 2560 / Math.max(bitmap.width, bitmap.height));
    if (scale === 1 && file.size < 1024 * 1024) return file;
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(bitmap.width * scale));
    canvas.height = Math.max(1, Math.round(bitmap.height * scale));
    const context = canvas.getContext("2d");
    if (!context) return file;
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    const type = file.type === "image/jpeg" ? "image/jpeg" : "image/webp";
    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, type, 0.9),
    );
    if (!blob || (blob.size >= file.size && scale === 1)) return file;
    const extension =
      blob.type === "image/webp"
        ? ".webp"
        : blob.type === "image/jpeg"
          ? ".jpg"
          : ".png";
    return new File([blob], file.name.replace(/\.[^.]+$/, "") + extension, {
      type: blob.type,
      lastModified: file.lastModified,
    });
  } finally {
    bitmap.close();
  }
}
