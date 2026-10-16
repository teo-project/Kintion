package records

// createRest — после записей пакета: значения (F-15), метки групп, участники
// с проверкой min/max (Д-22).
func (t *Tx) createRest(b Batch, res *BatchResult, refs *batchRefs) error {
	if err := t.createValues(b, res, refs); err != nil {
		return err
	}
	if err := t.createGroups(b, res); err != nil {
		return err
	}
	for _, r := range res.Records {
		if err := t.autoGroups(r); err != nil {
			return err
		}
	}
	return t.createParticipants(b, res, refs)
}
