package main

import (
	"fmt"
	"reflect"
)

type FoodService struct {

}

func (f *FoodService) SayName(request string, resp *string) error {

	*resp = "你点的菜是：" + request
	return nil
}

func (f *FoodService) SayName2(request string, resp *string) error {

	*resp = "你点的菜是：" + request
	return nil
}

func main () {
	rcvr := new(FoodService)
	typ := reflect.TypeOf(rcvr)
	fmt.Printf("typ: %+v\n", typ)

	// NumMethod() 返回使用 Method 可访问的方法数。
	fmt.Printf("NumMethod: %d\n", typ.NumMethod())

	for m := 0; m < typ.NumMethod(); m++ {
		// Method 返回类型方法集中的第 i 个方法。
		// 如果 i 不在 [0, NumMethod()) 范围内，它会 panic。
		//
		// 对于非接口类型 T 或 *T，返回的 Method 的 Type 和 Func 字段描述了一个函数，其第一个参数是接收者，并且只有导出的方法可以访问。
		//
		// 对于接口类型，返回的Method的Type字段给出了方法签名，没有 receiver，Func字段为 nil。
		//
		// Methods 按字典顺序排序。
		method := typ.Method(m)
		mtype := method.Type  // 方法类型
		mname := method.Name  // 方法名称

		// NumIn 返回函数类型的输入参数计数。
		// 如果类型的 Kind 不是 Func，它会 panic。
		mnum := mtype.NumIn()

		// In 返回函数类型的第 i 个输入参数的类型。
		// 如果类型的 Kind 不是 Func，它会 panic。
		// 如果 i 不在 [0, NumIn()) 范围内，它会 panic。
		mIn1 :=mtype.In(1)

		// Kind 返回此类型的具体种类。
		mIn2 :=mtype.In(2)

		// NumOut 返回函数类型的输出参数计数。
		// 如果类型的 Kind 不是 Func，它会 panic。
		mout := mtype.NumOut()

		// Out 返回函数类型的第 i 个输出参数的类型。
		returnType := mtype.Out(0)

		fmt.Printf("method: %+v\n", method)
		fmt.Printf("method type: %+v\n", mtype)
		fmt.Printf("method name: %+v\n", mname)
		fmt.Printf("method mnum: %+v\n", mnum)
		fmt.Printf("method mIn1: %+v\n", mIn1)
		fmt.Printf("method mIn2: %+v\n", mIn2.Kind() == reflect.Ptr)
		fmt.Printf("method mout: %+v\n", mout)
		fmt.Printf("method returnType: %+v\n", returnType)




	}
}
