package typ

import "context"

// encodeCanonicalTest keeps quotient/emission laws on the sole scoped wire
// format without restoring the deleted ordinary production API.
func encodeCanonicalTest(ctx context.Context, value Type) ([]byte, error) {
	receipt, err := EncodeCanonicalFormals(ctx, value, nil)
	if err != nil {
		return nil, err
	}
	return receipt.Bytes(), nil
}

func encodeCanonicalWithEncoderTest(encoder *canonicalEncoder, ctx context.Context, value Type) ([]byte, error) {
	receipt, err := encoder.encodeFormals(ctx, value, nil)
	if err != nil {
		return nil, err
	}
	return receipt.Bytes(), nil
}

func digestCanonicalTest(ctx context.Context, value Type) (CanonicalDigest, error) {
	receipt, err := EncodeCanonicalFormals(ctx, value, nil)
	if err != nil {
		return CanonicalDigest{}, err
	}
	digest, _ := receipt.Digest()
	return digest, nil
}

func digestCanonicalFormalsTest(ctx context.Context, value Type, formals []*TypeParam) (CanonicalDigest, error) {
	receipt, err := EncodeCanonicalFormals(ctx, value, formals)
	if err != nil {
		return CanonicalDigest{}, err
	}
	digest, _ := receipt.Digest()
	return digest, nil
}
