package filters_test

import (
	"testing"

	. "github.com/eljuanchosf/firehose-to-syslog/filters"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEvents(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filters Suite")
}

var _ = Describe("Filters", func() {
	Describe("GetFilters", func() {
		fixtureFilterFile := "/home/juan/.gvm/pkgsets/go1.5.1/global/src/github.com/eljuanchosf/firehose-to-syslog/filters/fixtures/filters.yaml"
		filtersStruct := GetFilters(fixtureFilterFile)
		Context("gets the right structure", func() {
			It("should get some orgs", func() {
				Expect(filtersStruct.Orgs.CFFieldName).To(Equal("cf_org_name"))
			})
		})
	})
})
