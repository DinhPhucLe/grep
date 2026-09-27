import { useEffect, useId, useState } from 'react';
import { Paper, Stack, Text, TextInput, UnstyledButton } from '@mantine/core';

export type SearchSuggestion = {
  id: string;
  primary: string;
  secondary?: string;
};

interface DirectorySearchProps {
  label: string;
  placeholder: string;
  loadSuggestions: (query: string) => Promise<SearchSuggestion[]>;
  onSelect: (id: string) => void;
}

export function DirectorySearch({
  label,
  placeholder,
  loadSuggestions,
  onSelect,
}: DirectorySearchProps) {
  const listId = useId();
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<SearchSuggestion[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    let cancelled = false;
    const timer = window.setTimeout(() => {
      loadSuggestions(query)
        .then((next) => {
          if (!cancelled) {
            setItems(next);
            setError(null);
          }
        })
        .catch((err: unknown) => {
          if (!cancelled) {
            setItems([]);
            setError(err instanceof Error ? err.message : 'Search failed');
          }
        });
    }, 150);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [open, query, loadSuggestions]);

  return (
    <Stack gap={4} style={{ position: 'relative', minWidth: 220, flex: 1 }}>
      <TextInput
        label={label}
        placeholder={placeholder}
        value={query}
        onChange={(event) => setQuery(event.currentTarget.value)}
        onFocus={() => setOpen(true)}
        onBlur={() => {
          window.setTimeout(() => setOpen(false), 120);
        }}
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        styles={{
          label: { textTransform: 'uppercase', fontWeight: 700, fontSize: '0.7rem' },
          input: {
            borderWidth: 2,
            borderColor: '#000',
            borderRadius: 0,
            fontFamily: 'inherit',
          },
        }}
      />
      {open ? (
        <Paper
          id={listId}
          role="listbox"
          className="search-suggestions"
          p="xs"
          radius={0}
          withBorder
          style={{ position: 'absolute', top: '100%', left: 0, right: 0, zIndex: 20 }}
        >
          {error ? (
            <Text fz="xs" c="red">
              {error}
            </Text>
          ) : null}
          {!error && items.length === 0 ? (
            <Text className="secondary-text" fz="xs">
              No matches
            </Text>
          ) : null}
          {items.map((item) => (
            <UnstyledButton
              key={item.id}
              role="option"
              className="search-suggestion"
              onMouseDown={(event) => {
                event.preventDefault();
                onSelect(item.id);
              }}
              w="100%"
              p="xs"
            >
              <Text fw={700} fz="sm">
                {item.primary}
              </Text>
              {item.secondary ? (
                <Text className="secondary-text" fz="xs">
                  {item.secondary}
                </Text>
              ) : null}
            </UnstyledButton>
          ))}
        </Paper>
      ) : null}
    </Stack>
  );
}
