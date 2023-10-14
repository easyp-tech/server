package core

import (
	"io/fs"
	"time"

	"github.com/gofrs/uuid"
)

type (
	// Repository represents
	Repository struct {
		fs.FS
		Owner      string
		Repository string
		Branch     string
		Commit     string
		CreatedAt  time.Time
		UpdatedAt  time.Time
	}

	// GetRequest contains git info for getting repository.
	GetRequest struct {
		Owner      string
		Repository string
		// If empty use default.
		Branch string
	}

	// SearchParams params for search users.
	SearchParams struct {
		OwnerID  uuid.UUID
		FullName string
		Limit    uint
		Offset   uint
	}

	// User contains user information.
	User struct {
		ID           uuid.UUID
		Email        string
		PassHash     []byte
		FullName     string
		Telegram     string
		Shop         string
		Experience   UserExperience
		Markets      []UserMarket
		Products     []UserProducts
		MarketInfo   string
		CreatedAt    time.Time
		UpdatedAt    time.Time
		OwnRole      OwnRole
		SKU          SKU
		TotalMarkets TotalMarkets
		Phone        string
	}

	Verify struct {
		Code       string
		UserID     uuid.UUID
		CreatedAt  time.Time
		VerifyType VerifyType
		Data       string
	}

	// UserExperience user experience.
	UserExperience uint8

	// UserMarket user market.
	UserMarket uint8

	// UserProducts user product.
	UserProducts uint8

	// VerifyType verify type.
	VerifyType uint8

	// OwnRole type.
	OwnRole uint8

	// SKU type.
	SKU uint8

	//TotalMarkets type.
	TotalMarkets uint8
)

//go:generate stringer -output=stringer.UserExperience.go -type=UserExperience -trimprefix=UserExperience
const (
	_ UserExperience = iota
	UserExperienceLessThen3Month
	UserExperienceBetween3MonthAnd1Year
	UserExperienceGreaterThen1Year
	UserExperienceUnknown
	UserExperienceNotYetWork
	UserExperienceLessThen6Month
	UserExperienceLessThen1Year
	UserExperienceBetween1And3Years
	UserExperienceBetween3And5Years
	UserExperienceMoreThen5Years
)

//go:generate stringer -output=stringer.UserMarket.go -type=UserMarket -trimprefix=UserMarket
const (
	_ UserMarket = iota
	UserMarketOzon
	UserMarketWildBerries
	UserMarketYandex
	UserMarketSberMega
	UserMarketAliexpress
	UserMarketAnother
)

//go:generate stringer -output=stringer.UserProducts.go -type=UserProducts -trimprefix=UserProducts
const (
	_ UserProducts = iota
	UserProductsCalculator
	UserProductsMailing
)

//go:generate stringer -output=stringer.VerifyType.go -type=VerifyType -trimprefix=VerifyType
const (
	_ VerifyType = iota
	VerifyTypeOnboard
	VerifyTypeEmailReset
)

const (
	SubjectRegistration  = `Подтверждение регистрации`
	SubjectPasswordReset = `Восстановление пароля`
	SubjectChangePass    = `Ваш пароль был изменен`
)

//go:generate stringer -output=stringer.OwnRole.go -type=OwnRole -trimprefix=OwnRole
const (
	_ OwnRole = iota
	OwnRoleIWantToSellOnMarketplaces
	OwnRoleISellItMyself
	OwnRoleIWorkInACompany
	OwnRoleFoundedMyCompany
	OwnRoleUnknown
)

//go:generate stringer -output=stringer.SKU.go -type=SKU -trimprefix=SKU
const (
	_ SKU = iota
	SKUSeveral
	SKULessThen50
	SKUBetween50And100
	SKUBetween100And300
	SKUBetween300And500
	SKUBetween500And1000
	SKUBetween1000And3000
	SKUMoreThen3000
	SKUUnknown
)

//go:generate stringer -output=stringer.TotalMarkets.go -type=TotalMarkets -trimprefix=TotalMarkets
const (
	_ TotalMarkets = iota
	TotalMarketsNotYet
	TotalMarketsOne
	TotalMarketsTwoOrThree
	TotalMarketsBetweenFourAndTen
	TotalMarketsMoreThenTen
	TotalMarketsUnknown
)
