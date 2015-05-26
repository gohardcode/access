package access

import (
	"fmt"
	"reflect"
)

var (
	fieldReaderInterface = reflect.TypeOf((*FieldReader)(nil)).Elem()
	fieldWriterInterface = reflect.TypeOf((*FieldWriter)(nil)).Elem()
)

type FieldReader interface {
	Field(string) (interface{}, error)
}

type FieldWriter interface {
	FieldReader
	SetField(string, interface{}) error
}

func readField(v reflect.Value, field string, path *Path) (reflect.Value, error) {

	v = indirect(v, fieldReaderInterface)

	vt := v.Type()
	vf := reflect.ValueOf(field)

	if r, ok := v.Interface().(FieldReader); ok {
		val, err := r.Field(field)
		fv := reflect.ValueOf(val)
		if err != nil {
			return fv, err
		}
		if path != nil {
			return path.read(fv)
		}
		return fv, err
	}

	switch vt.Kind() {
	case reflect.Interface:
		return readField(v.Elem(), field, path)
	case reflect.Map:

		if kk := vt.Key().Kind(); kk != reflect.String {
			return reflect.Value{}, fmt.Errorf("Map key type is not a string")
		}

		fv := v.MapIndex(vf)

		if !fv.IsValid() {
			return fv, fmt.Errorf("Map key not exists")
		}

		if path == nil {
			return fv, nil
		}

		return path.read(fv)

	case reflect.Struct:

		field = camelcased(field)

		if ft, ok := vt.FieldByName(field); ok {
			fv := v.FieldByIndex(ft.Index)

			if path == nil {
				return fv, nil
			}

			return path.read(fv)
		}

		if v.CanAddr() {
			v = v.Addr()
		}

		methods := []string{field, "Get" + field}
		for _, m := range methods {

			if mv := v.MethodByName(m); mv.IsValid() {

				if mt := mv.Type(); mt.NumIn() != 0 || mt.NumOut() != 1 {
					continue
				}

				fv := mv.Call([]reflect.Value{})[0]

				if path == nil {
					return fv, nil
				}
				return path.read(fv)
			}
		}

		return reflect.Value{}, fmt.Errorf("Struct has no field %s nor methods %v which satisfy signature func(...) (interface{}) ", camelcased, methods)
	default:
		return reflect.Value{}, fmt.Errorf("struct,map or FieldReader instance expected")
	}
}

func writeField(v reflect.Value, field string, path *Path, w reflect.Value, wt reflect.Type) error {

	v = indirect(v, fieldWriterInterface)

	if r, ok := v.Interface().(FieldWriter); ok {

		if path != nil {
			val, err := r.Field(field)
			if err != nil {
				return err
			}
			fv := makeAddressable(reflect.ValueOf(val))
			if err := path.write(fv, w, wt); err != nil {
				return err
			}
			return writeField(v, field, nil, fv, fv.Type())
		}

		return r.SetField(field, w.Interface())
	}

	vt := v.Type()
	vf := reflect.ValueOf(field)

	switch vt.Kind() {
	case reflect.Interface:

		var e reflect.Value

		if v.IsNil() && v.NumMethod() == 0 {
			e = reflect.ValueOf(map[string]interface{}{})
		} else {
			e = v.Elem()
			e = makeAddressable(e)
		}

		err := writeField(e, field, path, w, wt)

		if err == nil {
			v.Set(e)
		}

		return err

	case reflect.Map:

		if kk := vt.Key().Kind(); kk != reflect.String {
			return fmt.Errorf("Map key type is not a string")
		}

		if path != nil {
			fv := v.MapIndex(vf)
			if !fv.IsValid() {
				fv = reflect.New(vt.Elem()).Elem()
			}

			fv = makeAddressable(fv)

			if err := path.write(fv, w, wt); err != nil {
				return err
			}

			return writeField(v, field, nil, fv, fv.Type())
		}

		v.SetMapIndex(vf, w)
		return nil

	case reflect.Struct:

		field = camelcased(field)

		if ft, ok := vt.FieldByName(field); ok {
			fv := v.FieldByIndex(ft.Index)
			if path != nil {
				return path.write(fv, w, wt)
			}

			fv.Set(w)
			return nil
		}

		if v.CanAddr() {
			v = v.Addr()
		}

		if path != nil {
			methods := []string{field, "Get" + field}
			for _, m := range methods {

				if mv := v.MethodByName(m); mv.IsValid() {

					if mt := mv.Type(); mt.NumIn() != 0 || mt.NumOut() != 0 {
						continue
					}

					fv := mv.Call([]reflect.Value{})[0]

					if err := path.write(fv, w, wt); err != nil {
						return err
					}
					return writeField(v, field, nil, fv, fv.Type())
				}
			}

			return fmt.Errorf("Struct has no field %s nor methods %v which satisfy signature func() (interface{}) ", camelcased, methods)
		}

		methods := []string{field, "Set" + field}
		for _, m := range methods {

			if mv := v.MethodByName(m); mv.IsValid() {

				mt := mv.Type()
				numIn := mt.NumIn()

				if numIn > 2 || numIn == 0 || (numIn == 2 && !mt.IsVariadic()) {
					continue
				}

				if !wt.ConvertibleTo(mt.In(0)) {
					return fmt.Errorf("Can't call %s(%s)", m, wt)
				}

				mv.Call([]reflect.Value{w})
				return nil
			}
		}
		return fmt.Errorf("Struct has no field %s nor methods %v which satisfy signature func(interface{},...) (...) ", camelcased, methods)

	default:
		return fmt.Errorf("struct,map or FieldWriter instance expected")
	}
}
