// Hook for uploading assets to a node via multipart form data.

import { createSignal } from 'solid-js';
import type { UploadNodeAssetResponse } from '@sdk/types.gen';

// Client-side limits for quick feedback. Backend enforces these authoritatively.
const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB

const ALLOWED_MIME_TYPES = new Set([
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
  'image/svg+xml',
  'image/avif',
  'application/pdf',
]);

export interface UploadResult {
  name: string;
  mimeType: string;
  url: string;
}

export interface UseAssetUploadOptions {
  wsId: string;
  nodeId: string;
  getToken: () => string | null;
}

export interface UseAssetUploadReturn {
  uploadFile: (file: File) => Promise<UploadResult | null>;
  uploading: () => boolean;
  error: () => string | null;
}

export function useAssetUpload(options: UseAssetUploadOptions): UseAssetUploadReturn {
  const [uploading, setUploading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  const uploadFile = async (file: File): Promise<UploadResult | null> => {
    setError(null);

    // Validate file size
    if (file.size > MAX_FILE_SIZE) {
      const err = `File too large: ${file.name} (max 10MB)`;
      setError(err);
      console.error(err);
      return null;
    }

    // Validate MIME type
    if (!ALLOWED_MIME_TYPES.has(file.type)) {
      const err = `Unsupported file type: ${file.type}`;
      setError(err);
      console.error(err);
      return null;
    }

    const token = options.getToken();
    if (!token) {
      const err = 'Not authenticated';
      setError(err);
      console.error(err);
      return null;
    }

    setUploading(true);
    try {
      const formData = new FormData();
      formData.append('file', file);

      const response = await fetch(`/api/v1/workspaces/${options.wsId}/nodes/${options.nodeId}/assets`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${token}`,
        },
        body: formData,
      });

      if (!response.ok) {
        const errText = await response.text();
        throw new Error(`Upload failed: ${response.status} ${errText}`);
      }

      const result: UploadNodeAssetResponse = await response.json();
      return {
        name: result.name,
        mimeType: result.mime_type,
        url: result.url,
      };
    } catch (err) {
      const errMsg = err instanceof Error ? err.message : String(err);
      setError(errMsg);
      console.error('Asset upload failed:', err);
      return null;
    } finally {
      setUploading(false);
    }
  };

  return {
    uploadFile,
    uploading,
    error,
  };
}

/** Check if a MIME type represents an image */
export function isImageMimeType(mimeType: string): boolean {
  return mimeType.startsWith('image/');
}
