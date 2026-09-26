import { Button, Group, ScrollArea } from '@mantine/core';
import type { HeatmapFile } from '../../contracts/dashboard';

interface FileSelectorProps {
  files: HeatmapFile[];
  selectedPath: string;
  onSelect: (path: string) => void;
}

const basename = (path: string) => path.split('/').at(-1) ?? path;

export function FileSelector({ files, selectedPath, onSelect }: FileSelectorProps) {
  return (
    <ScrollArea type="auto" scrollbarSize={6}>
      <Group gap="xs" wrap="nowrap" pb="xs" role="tablist" aria-label="Changed files">
        {files.map((file) => {
          const selected = file.path === selectedPath;
          return (
            <Button
              key={file.path}
              role="tab"
              aria-selected={selected}
              title={file.path}
              variant={selected ? 'light' : 'subtle'}
              color={selected ? 'cyan' : 'gray'}
              size="compact-sm"
              onClick={() => onSelect(file.path)}
            >
              {basename(file.path)}
            </Button>
          );
        })}
      </Group>
    </ScrollArea>
  );
}
