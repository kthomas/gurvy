package bls381

import (
	"math/big"

	"github.com/consensys/gurvy"
	"github.com/consensys/gurvy/bls381/fp"
	"github.com/consensys/gurvy/bls381/fr"
	"github.com/consensys/gurvy/bls381/internal/fptower"
	"github.com/consensys/gurvy/utils"
)

// E: y**2=x**3+4
// Etwist: y**2 = x**3+4*(u+1)
// Tower: Fp->Fp2, u**2=-1 -> Fp12, v**6=u+1
// Generator (BLS12 family): x=-15132376222941642752
// optimal Ate loop: trace(frob)-1=x
// trace of pi: x+1
// Fp: p=4002409555221667393417789825735904156556882819939007885332058136124031650490837864442687629129015664037894272559787
// Fr: r=52435875175126190479447740508185965837690552500527637822603658699938581184513 (x**4-x**2+1)

// ID bls381 ID
const ID = gurvy.BLS381

// bCurveCoeff b coeff of the curve
var bCurveCoeff fp.Element

// bTwistCurveCoeff b coeff of the twist (defined over Fp2) curve
var bTwistCurveCoeff fptower.E2

// generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
var g1Gen G1Jac
var g2Gen G2Jac

var g1GenAff G1Affine
var g2GenAff G2Affine

// point at infinity
var g1Infinity G1Jac
var g2Infinity G2Jac

// optimal Ate loop counter (=trace-1 = x in BLS family)
var loopCounter [64]int8

// Parameters useful for the GLV scalar multiplication. The third roots define the
//  endomorphisms phi1 and phi2 for <G1Affine> and <G2Affine>. lambda is such that <r, phi-lambda> lies above
// <r> in the ring Z[phi]. More concretely it's the associated eigenvalue
// of phi1 (resp phi2) restricted to <G1Affine> (resp <G2Affine>)
// cf https://www.cosic.esat.kuleuven.be/nessie/reports/phase2/GLV.pdf
var thirdRootOneG1 fp.Element
var thirdRootOneG2 fp.Element
var lambdaGLV big.Int

// glvBasis stores R-linearly independant vectors (a,b), (c,d)
// in ker((u,v)->u+vlambda[r]), and their determinant
var glvBasis utils.Lattice

// psi o pi o psi**-1, where psi:E->E' is the degree 6 iso defined over Fp12
var endo struct {
	u fptower.E2
	v fptower.E2
}

// generator of the curve
var xGen big.Int

func init() {

	bCurveCoeff.SetUint64(4)
	bTwistCurveCoeff.A0.SetUint64(1)
	bTwistCurveCoeff.A1.SetUint64(1)
	bTwistCurveCoeff.MulByElement(&bTwistCurveCoeff, &bCurveCoeff)

	g1Gen.X.SetString("2407661716269791519325591009883849385849641130669941829988413640673772478386903154468379397813974815295049686961384")
	g1Gen.Y.SetString("821462058248938975967615814494474302717441302457255475448080663619194518120412959273482223614332657512049995916067")
	g1Gen.Z.SetString("1")

	g2Gen.X.SetString("3914881020997020027725320596272602335133880006033342744016315347583472833929664105802124952724390025419912690116411",
		"277275454976865553761595788585036366131740173742845697399904006633521909118147462773311856983264184840438626176168")
	g2Gen.Y.SetString("253800087101532902362860387055050889666401414686580130872654083467859828854605749525591159464755920666929166876282",
		"1710145663789443622734372402738721070158916073226464929008132596760920130516982819361355832232719175024697380252309")
	g2Gen.Z.SetString("1",
		"0")

	g1GenAff.FromJacobian(&g1Gen)
	g2GenAff.FromJacobian(&g2Gen)

	g1Infinity.X.SetOne()
	g1Infinity.Y.SetOne()
	g2Infinity.X.SetOne()
	g2Infinity.Y.SetOne()

	thirdRootOneG1.SetString("4002409555221667392624310435006688643935503118305586438271171395842971157480381377015405980053539358417135540939436")
	thirdRootOneG2.Square(&thirdRootOneG1)
	lambdaGLV.SetString("228988810152649578064853576960394133503", 10) //(x**2-1)
	_r := fr.Modulus()
	utils.PrecomputeLattice(_r, &lambdaGLV, &glvBasis)

	endo.u.A0.SetString("0")
	endo.u.A1.SetString("4002409555221667392624310435006688643935503118305586438271171395842971157480381377015405980053539358417135540939437")
	endo.v.A0.SetString("2973677408986561043442465346520108879172042883009249989176415018091420807192182638567116318576472649347015917690530")
	endo.v.A1.SetString("1028732146235106349975324479215795277384839936929757896155643118032610843298655225875571310552543014690878354869257")

	// binary decomposition of 15132376222941642752 little endian
	loopCounter = [64]int8{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 1, 1}

	xGen.SetString("15132376222941642752", 10)

}

// Generators return the generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
func Generators() (g1Jac G1Jac, g2Jac G2Jac, g1Aff G1Affine, g2Aff G2Affine) {
	g1Aff = g1GenAff
	g2Aff = g2GenAff
	g1Jac = g1Gen
	g2Jac = g2Gen
	return
}
