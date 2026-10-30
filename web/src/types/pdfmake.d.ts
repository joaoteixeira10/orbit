// pdfmake 0.3 ships no types: what lib/pdf.ts uses of it.
declare module 'pdfmake/build/pdfmake' {
  const pdfMake: {
    addVirtualFileSystem(vfs: Record<string, string>): void
    fonts: Record<string, Record<string, string>>
    createPdf(doc: unknown): { download(name: string): Promise<void>; getBlob(): Promise<Blob> }
  }
  export default pdfMake
}
declare module 'pdfmake/build/vfs_fonts' {
  const vfs: Record<string, string>
  export default vfs
}
