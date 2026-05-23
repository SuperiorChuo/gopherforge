import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const readSource = (relativePath: string) =>
  readFileSync(fileURLToPath(new URL(relativePath, import.meta.url)), 'utf8');

const extractUploadRequest = (source: string) => {
  const start = source.indexOf('const handleUploadRequest = async');
  const end = source.indexOf('\nconst handleUploadSuccess', start);
  return start >= 0 && end > start ? source.slice(start, end) : '';
};

describe('system file hash upload contract', () => {
  const apiSource = () => readSource('../../../api/system/file.ts');
  const pageSource = () => readSource('./index.vue');

  it('computes a SHA-256 hash with Web Crypto before upload', () => {
    const source = pageSource();

    expect(source).toMatch(/const calculateFileSha256 = async \(file: File\)/);
    expect(source).toContain("crypto.subtle.digest('SHA-256', buffer)");
    expect(source).toMatch(/await file\.arrayBuffer\(\)/);
    expect(source).toMatch(/catch[\s\S]*return ''/);
  });

  it('checks file hash before deciding whether to upload the real file', () => {
    const source = pageSource();

    expect(source).toContain(':request-method="handleUploadRequest"');
    expect(source).toMatch(/import \{[\s\S]*checkFileHash[\s\S]*uploadFile[\s\S]*\} from '@\/api\/system\/file';/);
    expect(source).toMatch(/const handleUploadRequest = async \(uploadFileInfo: UploadFile \| UploadFile\[\]\)/);
    expect(source).toMatch(/hash = await calculateFileSha256\(rawFile\);[\s\S]*const hashCheck = await checkFileHash\(hash\);/);
  });

  it('short-circuits upload when the hash already exists and refreshes data', () => {
    const source = pageSource();
    const requestMethod = extractUploadRequest(source);

    expect(requestMethod).toMatch(/if \(hashCheck\.exists && hashCheck\.file\)/);
    expect(requestMethod).toContain('MessagePlugin.success');
    expect(requestMethod).toMatch(/loadData\(\);[\s\S]*loadStats\(\);/);
    expect(source).toContain('const buildUploadResponse = (file: FileItem, instant = false) => ({');
    expect(source).toContain("status: 'success'");
    expect(source).toContain('instant,');
    expect(requestMethod).toContain('return buildUploadResponse(hashCheck.file, true);');

    const hitBranchStart = requestMethod.indexOf('if (hashCheck.exists && hashCheck.file)');
    const hitBranchEnd = requestMethod.indexOf('return buildUploadResponse(hashCheck.file, true);', hitBranchStart);
    const hitBranch = requestMethod.slice(hitBranchStart, hitBranchEnd);
    expect(hitBranch).not.toContain('uploadFile(');
  });

  it('continues with real upload when no hash match is found or hash cannot be computed', () => {
    const page = pageSource();
    const api = apiSource();
    const requestMethod = extractUploadRequest(page);

    expect(requestMethod).toMatch(/const uploaded = await uploadFile\(rawFile, hash \|\| undefined\);/);
    expect(requestMethod).toMatch(/catch[\s\S]*const uploaded = await uploadFile\(rawFile, hash \|\| undefined\);/);
    expect(api).toMatch(/export function uploadFile\(file: File, hash\?: string\)/);
    expect(api).toContain('if (hash) {');
    expect(api).toContain("formData.append('hash', hash);");
  });
});
