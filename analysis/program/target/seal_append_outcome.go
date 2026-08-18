package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func (c *Contract) appendProduced(input []producedDraft, callbacks []vocabulary.CallbackID) (indexRange, error) {
	rangeOut, err := checkedStoredRange("produced operation table", len(c.produced), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, produced := range input {
		captures, captureErr := checkedStoredRange("produced capture table", len(c.captures), len(produced.captures))
		if captureErr != nil {
			return indexRange{}, captureErr
		}
		for _, capture := range produced.captures {
			ordinal := capture.Ordinal
			if capture.Kind == vocabulary.CaptureCallback {
				ordinal = uint32(callbacks[capture.Ordinal-1])
			}
			c.captures = append(c.captures, captureRow{kind: capture.Kind, ordinal: ordinal})
		}
		typeValueCapture := noTypeValueCapture
		for captureIndex, capture := range produced.captures {
			if capture.Kind == vocabulary.CaptureTypeValueFormal {
				typeValueCapture = uint32(captureIndex)
				break
			}
		}
		c.produced = append(c.produced, producedRow{
			result: produced.result, target: produced.target, captures: captures, typeValueCapture: typeValueCapture,
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendFreshResults(input []freshResultDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("fresh result table", len(c.fresh), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, fresh := range input {
		c.fresh = append(c.fresh, freshResultRow(fresh))
	}
	return rangeOut, nil
}

func (c *Contract) appendCallbackResults(input []callbackResultDraft, callbacks []vocabulary.CallbackID) (indexRange, error) {
	rangeOut, err := checkedStoredRange("callback result table", len(c.callbackResults), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, result := range input {
		c.callbackResults = append(c.callbackResults, callbackResultRow{
			result: result.result, callback: callbacks[result.callback-1],
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendResultAliases(input []resultAliasDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("result alias table", len(c.resultAliases), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, alias := range input {
		c.resultAliases = append(c.resultAliases, resultAliasRow(alias))
	}
	return rangeOut, nil
}
