package rvfs

import (
	"fmt"
	"strings"
	"time"

	"github.com/buger/jsonparser"
)

// Parser extracts structure from Redfish JSON
type Parser struct{}

// NewParser creates a new parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse converts raw JSON into a Resource structure
func (p *Parser) Parse(path string, data []byte) (*Resource, error) {
	resource := &Resource{
		Path:       normalizePath(path),
		RawJSON:    data,
		Properties: make(map[string]*Property),
		Children:   make(map[string]*Child),
		FetchedAt:  time.Now(),
	}

	// Extract @odata.id and @odata.type
	if odataID, err := jsonparser.GetString(data, "@odata.id"); err == nil {
		resource.ODataID = odataID
	}
	if odataType, err := jsonparser.GetString(data, "@odata.type"); err == nil {
		resource.ODataType = odataType
	}

	// Parse properties and children
	err := jsonparser.ObjectEach(data, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		k := string(key)

		// Skip @odata.* metadata
		if strings.HasPrefix(k, "@odata.") {
			return nil
		}

		// Check if it's a child resource (object with ONLY @odata properties)
		if dataType == jsonparser.Object && p.isLinkOnly(value) {
			linkPath := p.extractODataID(value)
			childType := p.classifyLink(path, linkPath)
			resource.Children[k] = &Child{
				Name:   k,
				Type:   childType,
				Target: linkPath,
				Parent: path,
			}
			return nil
		}

		// Check for Members collection (special case for Children)
		if k == "Members" && dataType == jsonparser.Array && p.isLinkArray(value) {
			p.extractLinkArrayChildren(value, path, resource.Children)
			return nil
		}

		// Everything else is a property (parse recursively)
		prop := p.parseProperty(k, value, dataType)
		resource.Properties[k] = prop

		return nil
	})

	if err != nil {
		return nil, &ParseError{Path: path, Err: err}
	}

	return resource, nil
}

// parseProperty recursively parses a property into a tree structure
func (p *Parser) parseProperty(name string, value []byte, dataType jsonparser.ValueType) *Property {
	prop := &Property{
		Name:    name,
		RawJSON: value,
	}

	switch dataType {
	case jsonparser.Object:
		// Check if it's a link (only @-prefixed keys)
		if p.isLinkOnly(value) {
			prop.Type = PropertyLink
			prop.LinkTarget, _ = jsonparser.GetString(value, "@odata.id")
			return prop
		}

		// Regular object - recurse into children
		prop.Type = PropertyObject
		prop.Children = make(map[string]*Property)

		jsonparser.ObjectEach(value, func(childKey, childValue []byte, childType jsonparser.ValueType, offset int) error {
			k := string(childKey)

			// Skip OData metadata fields only (@odata.*)
			// Keep Redfish annotations (@Redfish.*), Message annotations (@Message.*), etc.
			if strings.HasPrefix(k, "@odata.") {
				return nil
			}

			// Recursive call
			childProp := p.parseProperty(k, childValue, childType)
			prop.Children[k] = childProp
			return nil
		})

	case jsonparser.Array:
		// Recurse into array elements
		prop.Type = PropertyArray
		prop.Elements = make([]*Property, 0)

		idx := 0
		jsonparser.ArrayEach(value, func(elemValue []byte, elemType jsonparser.ValueType, offset int, err error) {
			elemProp := p.parseProperty(fmt.Sprintf("[%d]", idx), elemValue, elemType)
			prop.Elements = append(prop.Elements, elemProp)
			idx++
		})

	case jsonparser.String:
		// Check if this string property is a URI reference by name convention
		if p.isURIProperty(name) {
			linkTarget, _ := jsonparser.ParseString(value)
			if strings.HasPrefix(linkTarget, "/") {
				prop.Type = PropertyLink
				prop.LinkTarget = linkTarget
				return prop
			}
		}
		// Regular string
		prop.Type = PropertySimple
		prop.Value = p.parseValue(value, dataType)

	default:
		// Number, bool, null
		prop.Type = PropertySimple
		prop.Value = p.parseValue(value, dataType)
	}

	return prop
}

// isURIProperty checks if a property name indicates a URI reference per DMTF spec.
// These string properties contain Redfish paths and should be treated as PropertyLinks.
func (p *Parser) isURIProperty(name string) bool {
	// DMTF spec: "Non-resource reference properties shall include the Uri or URI
	// term in their property name and shall be of type string."
	if strings.HasSuffix(name, "Uri") || strings.HasSuffix(name, "URI") {
		return true
	}
	// @Redfish.ActionInfo is always a URI string pointing to action info resource
	if name == "@Redfish.ActionInfo" {
		return true
	}
	// "target" in Actions context (action endpoint URI)
	if name == "target" {
		return true
	}
	return false
}

// isLinkOnly checks if JSON object contains ONLY OData metadata (no actual data)
// A link-only object has @odata.id and optionally other @odata.* fields, but no data properties
func (p *Parser) isLinkOnly(data []byte) bool {
	// Must have @odata.id
	if odataID, _ := jsonparser.GetString(data, "@odata.id"); odataID == "" {
		return false
	}

	// All keys must be OData metadata (@odata.*)
	// If we find any non-@odata.* key (including @Redfish.*, regular properties, etc.), it has data
	hasData := false
	jsonparser.ObjectEach(data, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		k := string(key)
		if !strings.HasPrefix(k, "@odata.") {
			hasData = true
		}
		return nil
	})

	return !hasData
}

// isLinkArray checks if JSON array contains only OData links
func (p *Parser) isLinkArray(data []byte) bool {
	if len(data) == 0 || data[0] != '[' {
		return false
	}

	allLinks := true
	count := 0

	jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		count++
		if dataType != jsonparser.Object || !p.isLinkOnly(value) {
			allLinks = false
		}
	})

	return count > 0 && allLinks
}

// extractODataID extracts @odata.id from JSON object
func (p *Parser) extractODataID(data []byte) string {
	if len(data) == 0 || data[0] != '{' {
		return ""
	}
	odataID, _ := jsonparser.GetString(data, "@odata.id")
	return odataID
}

// extractLinkArrayChildren extracts child names from link array
func (p *Parser) extractLinkArrayChildren(data []byte, parentPath string, children map[string]*Child) {
	jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if dataType == jsonparser.Object {
			if linkPath := p.extractODataID(value); linkPath != "" {
				// Extract name from link path
				name := p.extractNameFromPath(linkPath)
				if name != "" {
					childType := p.classifyLink(parentPath, linkPath)
					children[name] = &Child{
						Name:   name,
						Type:   childType,
						Target: linkPath,
						Parent: parentPath,
					}
				}
			}
		}
	})
}

// extractNameFromPath extracts the last segment of a path
func (p *Parser) extractNameFromPath(linkPath string) string {
	return BaseName(linkPath)
}

// classifyLink determines if a link is a child or external symlink
func (p *Parser) classifyLink(parentPath, targetPath string) ChildType {
	parent := strings.TrimRight(parentPath, "/")
	target := strings.TrimRight(targetPath, "/")

	// Same resource or child resource
	if target == parent || strings.HasPrefix(target, parent+"/") {
		return ChildLink
	}

	// External resource
	return ChildSymlink
}

// parseValue converts jsonparser value to Go value
func (p *Parser) parseValue(value []byte, dataType jsonparser.ValueType) interface{} {
	switch dataType {
	case jsonparser.String:
		s, _ := jsonparser.ParseString(value)
		return s
	case jsonparser.Number:
		f, _ := jsonparser.GetFloat(value)
		return f
	case jsonparser.Boolean:
		b, _ := jsonparser.GetBoolean(value)
		return b
	case jsonparser.Null:
		return nil
	case jsonparser.Object, jsonparser.Array:
		// Return raw JSON for complex types
		return value
	default:
		return string(value)
	}
}

// normalizePath ensures path starts with / and has no trailing /
func normalizePath(path string) string {
	if path == "" {
		return "/redfish/v1"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}
