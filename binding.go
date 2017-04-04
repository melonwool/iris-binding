package binding

import (
	"gopkg.in/kataras/iris.v6"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

func Form(context *iris.Context, formStruct interface{}, ifacePtr ...interface{}) (interface{}, int, Errors) {
	req := context.Request
	ensureNotPointer(formStruct)
	formStructs := reflect.New(reflect.TypeOf(formStruct))
	parseErr := req.ParseForm()
	var errors Errors

	if parseErr != nil {
		errors.Add([]string{}, DeserializationError, parseErr.Error())
	}
	mapForm(formStructs, req.Form, nil, errors)
	Validate(context, formStructs)
	return formStructs.Elem().Interface(), errors.Len(), errors
}

func Validate(context *iris.Context, obj interface{}) {
	req := context.Request
	var errors Errors

	v := reflect.ValueOf(obj)
	k := v.Kind()

	if k == reflect.Interface || k == reflect.Ptr {

		v = v.Elem()
		k = v.Kind()
	}

	if k == reflect.Slice || k == reflect.Array {

		for i := 0; i < v.Len(); i++ {

			e := v.Index(i).Interface()
			errors = validateStruct(errors, e)

			if validator, ok := e.(Validator); ok {
				errors = validator.Validate(errors, req)
			}
		}
	} else {

		errors = validateStruct(errors, obj)

		if validator, ok := obj.(Validator); ok {
			errors = validator.Validate(errors, req)
		}
	}
}

// Performs required field checking on a struct
func validateStruct(errors Errors, obj interface{}) Errors {
	typ := reflect.TypeOf(obj)
	val := reflect.ValueOf(obj)

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Skip ignored and unexported fields in the struct
		if field.Tag.Get("form") == "-" || !val.Field(i).CanInterface() {
			continue
		}

		fieldValue := val.Field(i).Interface()
		zero := reflect.Zero(field.Type).Interface()

		// Validate nested and embedded structs (if pointer, only do so if not nil)
		if field.Type.Kind() == reflect.Struct ||
			(field.Type.Kind() == reflect.Ptr && !reflect.DeepEqual(zero, fieldValue) &&
				field.Type.Elem().Kind() == reflect.Struct) {
			errors = validateStruct(errors, fieldValue)
		}

		if strings.Index(field.Tag.Get("binding"), "required") > -1 {
			if reflect.DeepEqual(zero, fieldValue) {
				name := field.Name
				if j := field.Tag.Get("json"); j != "" {
					name = j
				} else if f := field.Tag.Get("form"); f != "" {
					name = f
				}
				errors.Add([]string{name}, RequiredError, "Required")
			}
		}
	}
	return errors
}

// Takes values from the form data and puts them into a struct
func mapForm(formStruct reflect.Value, form map[string][]string,
	formfile map[string][]*multipart.FileHeader, errors Errors) {

	if formStruct.Kind() == reflect.Ptr {
		formStruct = formStruct.Elem()
	}
	typ := formStruct.Type()

	for i := 0; i < typ.NumField(); i++ {
		typeField := typ.Field(i)
		structField := formStruct.Field(i)

		if typeField.Type.Kind() == reflect.Ptr && typeField.Anonymous {
			structField.Set(reflect.New(typeField.Type.Elem()))
			mapForm(structField.Elem(), form, formfile, errors)
			if reflect.DeepEqual(structField.Elem().Interface(), reflect.Zero(structField.Elem().Type()).Interface()) {
				structField.Set(reflect.Zero(structField.Type()))
			}
		} else if typeField.Type.Kind() == reflect.Struct {
			mapForm(structField, form, formfile, errors)
		} else if inputFieldName := typeField.Tag.Get("form"); inputFieldName != "" {
			if !structField.CanSet() {
				continue
			}

			inputValue, exists := form[inputFieldName]
			inputFile, existsFile := formfile[inputFieldName]
			if exists && !existsFile {
				numElems := len(inputValue)
				if structField.Kind() == reflect.Slice && numElems > 0 {
					sliceOf := structField.Type().Elem().Kind()
					slice := reflect.MakeSlice(structField.Type(), numElems, numElems)
					for i := 0; i < numElems; i++ {
						setWithProperType(sliceOf, inputValue[i], slice.Index(i), inputFieldName, errors)
					}
					formStruct.Field(i).Set(slice)
				} else {
					kind := typeField.Type.Kind()
					if structField.Kind() == reflect.Ptr {
						structField.Set(reflect.New(typeField.Type.Elem()))
						structField = structField.Elem()
						kind = typeField.Type.Elem().Kind()
					}
					setWithProperType(kind, inputValue[0], structField, inputFieldName, errors)
				}
				continue
			}

			if !existsFile {
				continue
			}
			fhType := reflect.TypeOf((*multipart.FileHeader)(nil))
			numElems := len(inputFile)
			if structField.Kind() == reflect.Slice && numElems > 0 && structField.Type().Elem() == fhType {
				slice := reflect.MakeSlice(structField.Type(), numElems, numElems)
				for i := 0; i < numElems; i++ {
					slice.Index(i).Set(reflect.ValueOf(inputFile[i]))
				}
				structField.Set(slice)
			} else if structField.Type() == fhType {
				structField.Set(reflect.ValueOf(inputFile[0]))
			}
		}
	}
}

// This sets the value in a struct of an indeterminate type to the
// matching value from the request (via Form middleware) in the
// same type, so that not all deserialized values have to be strings.
// Supported types are string, int, float, and bool.
func setWithProperType(valueKind reflect.Kind, val string, structField reflect.Value, nameInTag string, errors Errors) {
	switch valueKind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val == "" {
			val = "0"
		}
		intVal, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			errors.Add([]string{nameInTag}, TypeError, "Value could not be parsed as integer")
		} else {
			structField.SetInt(intVal)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if val == "" {
			val = "0"
		}
		uintVal, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			errors.Add([]string{nameInTag}, TypeError, "Value could not be parsed as unsigned integer")
		} else {
			structField.SetUint(uintVal)
		}
	case reflect.Bool:
		if val == "" {
			val = "false"
		}
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			errors.Add([]string{nameInTag}, TypeError, "Value could not be parsed as boolean")
		} else {
			structField.SetBool(boolVal)
		}
	case reflect.Float32:
		if val == "" {
			val = "0.0"
		}
		floatVal, err := strconv.ParseFloat(val, 32)
		if err != nil {
			errors.Add([]string{nameInTag}, TypeError, "Value could not be parsed as 32-bit float")
		} else {
			structField.SetFloat(floatVal)
		}
	case reflect.Float64:
		if val == "" {
			val = "0.0"
		}
		floatVal, err := strconv.ParseFloat(val, 64)
		if err != nil {
			errors.Add([]string{nameInTag}, TypeError, "Value could not be parsed as 64-bit float")
		} else {
			structField.SetFloat(floatVal)
		}
	case reflect.String:
		structField.SetString(val)
	}
}

// Don't pass in pointers to bind to. Can lead to bugs. See:
// https://github.com/codegangsta/martini-contrib/pull/34#issuecomment-29683659
func ensureNotPointer(obj interface{}) {
	if reflect.TypeOf(obj).Kind() == reflect.Ptr {
		panic("Pointers are not accepted as binding models")
	}
}

type (
	// Implement the Validator interface to handle some rudimentary
	// request validation logic so your application doesn't have to.
	Validator interface {
		// Validate validates that the request is OK. It is recommended
		// that validation be limited to checking values for syntax and
		// semantics, enough to know that you can make sense of the request
		// in your application. For example, you might verify that a credit
		// card number matches a valid pattern, but you probably wouldn't
		// perform an actual credit card authorization here.
		Validate(Errors, *http.Request) Errors
	}
)
