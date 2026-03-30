CREATE UNIQUE INDEX ledger_entries_reference_type_reference_id_direction_entry_type_unique
  ON ledger_entries (reference_type, reference_id, direction, entry_type);
