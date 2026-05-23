import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const readSource = (relativePath: string) =>
  readFileSync(fileURLToPath(new URL(relativePath, import.meta.url)), 'utf8');

describe('system file API and preview contract', () => {
  const apiSource = () => readSource('../../../api/system/file.ts');
  const pageSource = () => readSource('./index.vue');

  it('aliases generated file schemas instead of keeping stale handwritten shapes', () => {
    const source = apiSource();

    expect(source).toContain("import type { components } from '@/api/generated/schema';");
    expect(source).toContain("type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];");
    expect(source).toContain("export type FileItem = Schema<'FileItem'>;");
    expect(source).toContain("export type FileListResponse = Schema<'FileListResponse'>;");
    expect(source).toContain("export type FileStats = Schema<'FileStats'>;");
    expect(source).toContain("export type MultipleUploadResponse = Schema<'MultipleUploadResponse'>;");
    expect(source).not.toContain('total_count');
    expect(source).not.toContain('Record<string, number | { count: number; size: number }>');
  });

  it('only enables preview for image files and falls thumbnails back to original image urls', () => {
    const source = pageSource();

    expect(source).toMatch(/const isImageFile = \(row: FileItem\) =>[\s\S]*row\.file_type[\s\S]*row\.mime_type/);
    expect(source).toContain("const fileThumbnail = (row: FileItem) => row.thumbnail_url || (isImageFile(row) ? row.url : '');");
    expect(source).toMatch(/const handlePreview = async \(row: FileItem\) => \{[\s\S]*if \(!isImageFile\(row\)\)/);
    expect(source).toContain('v-if="isImageFile(row)"');
    expect(source).not.toContain('<t-link theme="primary" hover="color" @click="handlePreview(row)">预览</t-link>');
  });
});
